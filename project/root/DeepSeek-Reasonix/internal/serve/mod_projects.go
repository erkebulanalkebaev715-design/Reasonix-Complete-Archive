package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/store"
)

const modProjectRegistrySchema = 1
const modProjectRegistryMax = 128

type modProjectRecord struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Workspace        string `json:"workspace"`
	CreatedAtUnixMS  int64  `json:"createdAtUnixMs"`
	UpdatedAtUnixMS  int64  `json:"updatedAtUnixMs"`
	LastOpenedUnixMS int64  `json:"lastOpenedUnixMs,omitempty"`
}

type modProjectRegistryState struct {
	SchemaVersion int                `json:"schemaVersion"`
	Projects      []modProjectRecord `json:"projects"`
}

type modProjectView struct {
	modProjectRecord
	Current    bool `json:"current"`
	Registered bool `json:"registered"`
	Available  bool `json:"available"`
}

func modProjectRegistryPath() string {
	base := strings.TrimSpace(config.ReasonixHomeDir())
	if base == "" {
		return ""
	}
	return filepath.Join(base, "balance", "projects.json")
}

func modProjectID(workspace string) string {
	sum := sha256.Sum256([]byte(canonicalModWorkspace(workspace)))
	return hex.EncodeToString(sum[:12])
}

func validateModProjectName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("project name is required")
	}
	if utf8.RuneCountInString(name) > 128 {
		return "", fmt.Errorf("project name is too long")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("project name contains control characters")
		}
	}
	return name, nil
}

func validateModProjectWorkspace(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("workspace is required")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("invalid workspace")
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("workspace not found")
	}
	st, err := os.Stat(real)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("workspace must be an existing directory")
	}
	return filepath.Clean(real), nil
}

func readModProjectRegistry(path string) (modProjectRegistryState, error) {
	state := modProjectRegistryState{SchemaVersion: modProjectRegistrySchema, Projects: []modProjectRecord{}}
	if path == "" {
		return state, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if len(data) > 2<<20 {
		return state, fmt.Errorf("project registry is too large")
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode project registry: %w", err)
	}
	if state.SchemaVersion != modProjectRegistrySchema {
		return state, fmt.Errorf("unsupported project registry schema %d", state.SchemaVersion)
	}
	seen := make(map[string]struct{}, len(state.Projects))
	for i := range state.Projects {
		p := &state.Projects[i]
		workspace := canonicalModWorkspace(p.Workspace)
		if strings.TrimSpace(workspace) == "" {
			return state, fmt.Errorf("project %q has empty workspace", p.ID)
		}
		p.Workspace = workspace
		if p.ID != modProjectID(workspace) {
			return state, fmt.Errorf("project %q has mismatched workspace identity", p.ID)
		}
		if _, ok := seen[p.ID]; ok {
			return state, fmt.Errorf("duplicate project id %q", p.ID)
		}
		seen[p.ID] = struct{}{}
		if _, err := validateModProjectName(p.Name); err != nil {
			return state, fmt.Errorf("project %q: %w", p.ID, err)
		}
	}
	return state, nil
}

func writeModProjectRegistry(path string, state modProjectRegistryState) error {
	if path == "" {
		return fmt.Errorf("Reasonix home is unavailable")
	}
	if len(state.Projects) > modProjectRegistryMax {
		return fmt.Errorf("project registry limit exceeded")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".projects-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Server) modProjectViews() ([]modProjectView, error) {
	s.modProjectsMu.Lock()
	defer s.modProjectsMu.Unlock()
	state, err := readModProjectRegistry(modProjectRegistryPath())
	if err != nil {
		return nil, err
	}
	current := canonicalModWorkspace(s.modWorkspacePath())
	out := make([]modProjectView, 0, len(state.Projects)+1)
	foundCurrent := false
	for _, p := range state.Projects {
		isCurrent := canonicalModWorkspace(p.Workspace) == current
		foundCurrent = foundCurrent || isCurrent
		st, statErr := os.Stat(p.Workspace)
		available := statErr == nil && st.IsDir()
		out = append(out, modProjectView{modProjectRecord: p, Current: isCurrent, Registered: true, Available: available})
	}
	if !foundCurrent && current != "" {
		name := filepath.Base(current)
		if name == "." || name == string(filepath.Separator) || strings.TrimSpace(name) == "" {
			name = "Current project"
		}
		st, statErr := os.Stat(current)
		available := statErr == nil && st.IsDir()
		out = append(out, modProjectView{modProjectRecord: modProjectRecord{ID: modProjectID(current), Name: name, Workspace: current}, Current: true, Registered: false, Available: available})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current
		}
		ai := out[i].LastOpenedUnixMS
		if ai == 0 {
			ai = out[i].UpdatedAtUnixMS
		}
		aj := out[j].LastOpenedUnixMS
		if aj == 0 {
			aj = out[j].UpdatedAtUnixMS
		}
		if ai != aj {
			return ai > aj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Server) modProjectsGet(w http.ResponseWriter, _ *http.Request) {
	projects, err := s.modProjectViews()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"projects":         projects,
		"count":            len(projects),
		"currentWorkspace": canonicalModWorkspace(s.modWorkspacePath()),
		"budgetScope":      "workspace",
		"workspaceSwitch":  "supervisor-restart",
	})
}

func (s *Server) modProjectsRegister(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot change project registry while a turn is running", http.StatusConflict)
		return
	}
	var req struct {
		Name      string `json:"name"`
		Workspace string `json:"workspace"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid project: "+err.Error(), http.StatusBadRequest)
		return
	}
	workspace, err := validateModProjectWorkspace(req.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(workspace)
	}
	name, err = validateModProjectName(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.modProjectsMu.Lock()
	defer s.modProjectsMu.Unlock()
	path := modProjectRegistryPath()
	state, err := readModProjectRegistry(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := modProjectID(workspace)
	now := time.Now().UnixMilli()
	found := false
	for i := range state.Projects {
		if state.Projects[i].ID == id {
			state.Projects[i].Name = name
			state.Projects[i].Workspace = workspace
			state.Projects[i].UpdatedAtUnixMS = now
			found = true
			break
		}
	}
	if !found {
		if len(state.Projects) >= modProjectRegistryMax {
			http.Error(w, "project registry limit reached", http.StatusConflict)
			return
		}
		state.Projects = append(state.Projects, modProjectRecord{ID: id, Name: name, Workspace: workspace, CreatedAtUnixMS: now, UpdatedAtUnixMS: now})
	}
	if err := writeModProjectRegistry(path, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.modHub.Emit("project.registered", map[string]any{"id": id, "workspace": workspace})
	writeJSON(w, map[string]any{"id": id, "name": name, "workspace": workspace, "current": canonicalModWorkspace(s.modWorkspacePath()) == workspace})
}

func (s *Server) modProjectsRemove(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot change project registry while a turn is running", http.StatusConflict)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.ID) == "" {
		http.Error(w, "project id is required", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(req.ID)
	s.modProjectsMu.Lock()
	defer s.modProjectsMu.Unlock()
	path := modProjectRegistryPath()
	state, err := readModProjectRegistry(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := state.Projects[:0]
	found := false
	for _, p := range state.Projects {
		if p.ID == id {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	state.Projects = out
	if err := writeModProjectRegistry(path, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.modHub.Emit("project.unregistered", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) modProjectsOpen(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "stop the current turn before switching projects", http.StatusConflict)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.ID) == "" {
		http.Error(w, "project id is required", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(req.ID)
	s.modProjectsMu.Lock()
	path := modProjectRegistryPath()
	state, err := readModProjectRegistry(path)
	if err != nil {
		s.modProjectsMu.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var selected *modProjectRecord
	now := time.Now().UnixMilli()
	for i := range state.Projects {
		if state.Projects[i].ID == id {
			state.Projects[i].LastOpenedUnixMS = now
			state.Projects[i].UpdatedAtUnixMS = now
			copy := state.Projects[i]
			selected = &copy
			break
		}
	}
	if selected == nil {
		s.modProjectsMu.Unlock()
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	workspace, validateErr := validateModProjectWorkspace(selected.Workspace)
	if validateErr != nil {
		s.modProjectsMu.Unlock()
		http.Error(w, "project workspace unavailable: "+validateErr.Error(), http.StatusConflict)
		return
	}
	selected.Workspace = workspace
	if err := writeModProjectRegistry(path, state); err != nil {
		s.modProjectsMu.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.modProjectsMu.Unlock()
	current := canonicalModWorkspace(s.modWorkspacePath())
	restart := selected.Workspace != current
	s.modHub.Emit("project.open.requested", map[string]any{"id": selected.ID, "workspace": selected.Workspace, "restartRequired": restart})
	writeJSON(w, map[string]any{
		"id": selected.ID, "name": selected.Name, "workspace": selected.Workspace,
		"currentWorkspace": current, "restartRequired": restart,
		"supervisor": map[string]any{"action": "restart", "workspace": selected.Workspace},
	})
}

type modTaskView struct {
	ID              string `json:"id"`
	Path            string `json:"path"`
	Title           string `json:"title"`
	Preview         string `json:"preview,omitempty"`
	Turns           int    `json:"turns"`
	CountsKnown     bool   `json:"countsKnown"`
	Current         bool   `json:"current"`
	CreatedAtUnixMS int64  `json:"createdAtUnixMs,omitempty"`
	UpdatedAtUnixMS int64  `json:"updatedAtUnixMs,omitempty"`
	Recovered       bool   `json:"recovered,omitempty"`
	RecoveryReason  string `json:"recoveryReason,omitempty"`
}

func (s *Server) modTaskViews() ([]modTaskView, error) {
	dir := strings.TrimSpace(s.ctl().SessionDir())
	if dir == "" {
		return []modTaskView{}, nil
	}
	items, err := agent.ListSessions(dir)
	if err != nil {
		return nil, err
	}
	current := filepath.Clean(s.ctl().SessionPath())
	out := make([]modTaskView, 0, len(items))
	for _, item := range items {
		if item.WorkspaceRoot != "" && canonicalModWorkspace(item.WorkspaceRoot) != canonicalModWorkspace(s.modWorkspacePath()) {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(item.Path), filepath.Ext(item.Path))
		title := strings.TrimSpace(item.CustomTitle)
		if title == "" {
			title = strings.TrimSpace(item.TopicTitle)
		}
		preview := modBoundedRedacted(strings.TrimSpace(item.Preview), 512)
		if title == "" {
			title = preview
		}
		if title == "" {
			title = id
		}
		createdMS, updatedMS := int64(0), int64(0)
		if !item.CreatedAt.IsZero() {
			createdMS = item.CreatedAt.UnixMilli()
		}
		if !item.LastActivityAt.IsZero() {
			updatedMS = item.LastActivityAt.UnixMilli()
		}
		out = append(out, modTaskView{
			ID: id, Path: item.Path, Title: modBoundedRedacted(title, 256), Preview: preview,
			Turns: item.Turns, CountsKnown: item.CountsKnown, Current: filepath.Clean(item.Path) == current,
			CreatedAtUnixMS: createdMS, UpdatedAtUnixMS: updatedMS,
			Recovered: item.Recovered, RecoveryReason: modBoundedRedacted(item.RecoveryReason, 256),
		})
	}
	return out, nil
}

func (s *Server) modTasksGet(w http.ResponseWriter, _ *http.Request) {
	tasks, err := s.modTaskViews()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"tasks": tasks, "count": len(tasks), "running": s.ctl().Running(),
		"nativeLifecycle":         map[string]string{"new": "/new", "resume": "/resume", "delete": "/delete-session"},
		"providerCallsForListing": false,
	})
}

func validateModTaskTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	if utf8.RuneCountInString(title) > 160 {
		return "", fmt.Errorf("title is too long")
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("title contains control characters")
		}
	}
	return title, nil
}

func modTaskPathWithinDir(dir, raw string) (string, error) {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("invalid task path")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("invalid session directory")
	}
	realDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return "", fmt.Errorf("invalid session directory")
	}
	absPath, err := filepath.Abs(strings.TrimSpace(raw))
	if err != nil || !store.IsSessionTranscriptName(filepath.Base(absPath)) {
		return "", fmt.Errorf("invalid task path")
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("task not found")
	}
	rel, err := filepath.Rel(realDir, realPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("task path outside session directory")
	}
	return realPath, nil
}

func (s *Server) modTasksRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path  string `json:"path"`
		Title string `json:"title"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid task rename: "+err.Error(), http.StatusBadRequest)
		return
	}
	path, err := modTaskPathWithinDir(s.ctl().SessionDir(), req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	title, err := validateModTaskTitle(req.Title)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := agent.RenameSession(path, title); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.modHub.Emit("task.renamed", map[string]any{"id": strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "title": title})
	writeJSON(w, map[string]any{"path": path, "title": title})
}
