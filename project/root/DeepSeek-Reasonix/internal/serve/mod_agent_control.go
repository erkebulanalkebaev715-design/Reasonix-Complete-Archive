package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"reasonix/internal/efficiency"
	"reasonix/internal/memory"
	"reasonix/internal/tool"
)

type modToolPolicyController interface {
	AllToolContractEntries() []tool.ContractEntry
	ToolContractEntries() []tool.ContractEntry
	SessionToolDecisions() map[string]string
	SetSessionToolDecisions(map[string]string) error
}

type modToolView struct {
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	ReadOnly        bool   `json:"readOnly"`
	ProviderVisible bool   `json:"providerVisible"`
	Decision        string `json:"decision"`
}

type modSkillView struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope"`
	RunAs       string `json:"runAs"`
	ReadOnly    bool   `json:"readOnly"`
	Enabled     bool   `json:"enabled"`
}

func (s *Server) modToolDecisionsSnapshot() map[string]string {
	s.modMu.RLock()
	defer s.modMu.RUnlock()
	out := make(map[string]string, len(s.modToolDecisions))
	for k, v := range s.modToolDecisions {
		out[k] = v
	}
	return out
}

func (s *Server) setModToolDecisions(v map[string]string) {
	s.modMu.Lock()
	defer s.modMu.Unlock()
	s.modToolDecisions = make(map[string]string, len(v))
	for k, x := range v {
		s.modToolDecisions[k] = x
	}
}

func (s *Server) canonicalModToolDecisions(in map[string]string) (map[string]string, error) {
	ctl, ok := s.ctl().(modToolPolicyController)
	if !ok {
		return nil, fmt.Errorf("runtime tool policy unavailable")
	}
	known := map[string]string{}
	for _, e := range ctl.AllToolContractEntries() {
		known[strings.ToLower(strings.TrimSpace(e.Name))] = e.Name
	}
	out := make(map[string]string, len(in))
	for raw, value := range in {
		name, ok := known[strings.ToLower(strings.TrimSpace(raw))]
		if !ok {
			return nil, fmt.Errorf("unknown tool: %s", strings.TrimSpace(raw))
		}
		decision := strings.ToLower(strings.TrimSpace(value))
		switch decision {
		case "allow", "ask", "deny":
		default:
			return nil, fmt.Errorf("invalid decision for %s: use allow, ask, or deny", name)
		}
		out[name] = decision
	}
	return out, nil
}

func (s *Server) modExecutionPolicyModeSnapshot() efficiency.ExecutionMode {
	s.modMu.RLock()
	defer s.modMu.RUnlock()
	return s.modExecutionPolicyMode
}

func (s *Server) setModExecutionPolicyMode(mode efficiency.ExecutionMode) {
	s.modMu.Lock()
	s.modExecutionPolicyMode = mode
	s.modMu.Unlock()
}

// prepareModExecutionPolicyMode installs the tool-policy overlay before a model
// switch. The returned undo closure restores the previous overlay if the switch
// fails. This keeps pro_diagnosis physically read-only even across controller
// rebuilds, because switchModel re-applies the same effective policy to the new
// controller before it can accept work.
func (s *Server) prepareModExecutionPolicyMode(mode efficiency.ExecutionMode) (func(), error) {
	previous := s.modExecutionPolicyModeSnapshot()
	s.setModExecutionPolicyMode(mode)
	if err := s.applyModToolDecisionsToCurrentController(); err != nil {
		s.setModExecutionPolicyMode(previous)
		_ = s.applyModToolDecisionsToCurrentController()
		return nil, err
	}
	return func() {
		s.setModExecutionPolicyMode(previous)
		_ = s.applyModToolDecisionsToCurrentController()
	}, nil
}

func (s *Server) applyModToolDecisionsToCurrentController() error {
	ctl, ok := s.ctl().(modToolPolicyController)
	if !ok {
		return fmt.Errorf("controller does not expose runtime tool policy")
	}
	return ctl.SetSessionToolDecisions(s.modEffectiveToolDecisions())
}

func (s *Server) modAgentReload(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot reload agent while a turn is running", http.StatusConflict)
		return
	}
	ref := currentModelRef(s.ctl())
	if strings.TrimSpace(ref) == "" {
		http.Error(w, "current model ref unavailable", http.StatusConflict)
		return
	}
	if err := s.switchModel(r.Context(), ref); err != nil {
		http.Error(w, "reload agent: "+err.Error(), http.StatusConflict)
		return
	}
	s.modHub.Emit("agent.reloaded", map[string]any{"modelRef": currentModelRef(s.ctl()), "workspace": s.ctl().WorkspaceRoot()})
	writeJSON(w, map[string]any{"ok": true, "modelRef": currentModelRef(s.ctl()), "workspace": s.ctl().WorkspaceRoot()})
}

func (s *Server) modAgentGet(w http.ResponseWriter, _ *http.Request) {
	ctl := s.ctl()
	writeJSON(w, map[string]any{
		"workspace": ctl.WorkspaceRoot(), "modelRef": ctl.ModelRef(), "running": ctl.Running(),
		"toolApprovalMode": ctl.ToolApprovalMode(), "tools": s.modToolViews(), "skills": s.modSkillViews(),
		"instructions": s.modInstructionViews(false), "profile": s.modProfile.Snapshot(),
		"environment": s.modEnvironmentSnapshot(context.Background(), false),
	})
}

func (s *Server) modToolViews() []modToolView {
	ctl, ok := s.ctl().(modToolPolicyController)
	if !ok {
		return nil
	}
	visible := map[string]bool{}
	for _, e := range ctl.ToolContractEntries() {
		visible[e.Name] = true
	}
	effective := ctl.SessionToolDecisions()
	out := []modToolView{}
	for _, e := range ctl.AllToolContractEntries() {
		d := effective[e.Name]
		if d == "" {
			d = "inherit"
		}
		out = append(out, modToolView{Name: e.Name, Description: e.Description, ReadOnly: e.ReadOnly, ProviderVisible: visible[e.Name], Decision: d})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Server) modAgentToolsGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"tools": s.modToolViews(), "approvalMode": s.ctl().ToolApprovalMode()})
}

func (s *Server) modAgentToolsSet(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot change tools while a turn is running", http.StatusConflict)
		return
	}
	var body struct {
		Decisions    map[string]string `json:"decisions"`
		ApprovalMode string            `json:"approvalMode,omitempty"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "invalid tool policy: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Decisions == nil {
		body.Decisions = map[string]string{}
	}
	mode := strings.ToLower(strings.TrimSpace(body.ApprovalMode))
	if mode != "" && mode != "ask" && mode != "auto" && mode != "yolo" {
		http.Error(w, "approvalMode must be ask, auto, or yolo", http.StatusBadRequest)
		return
	}
	if _, ok := s.ctl().(modToolPolicyController); !ok {
		http.Error(w, "runtime tool policy unavailable", http.StatusNotImplemented)
		return
	}
	canonical, err := s.canonicalModToolDecisions(body.Decisions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	previous := s.modToolDecisionsSnapshot()
	s.setModToolDecisions(canonical)
	if err := s.applyModToolDecisionsToCurrentController(); err != nil {
		s.setModToolDecisions(previous)
		_ = s.applyModToolDecisionsToCurrentController()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if mode != "" {
		s.ctl().SetToolApprovalMode(mode)
	}
	payload := map[string]any{"tools": s.modToolViews(), "approvalMode": s.ctl().ToolApprovalMode()}
	s.modHub.Emit("agent.tools.updated", map[string]any{"count": len(body.Decisions), "approvalMode": s.ctl().ToolApprovalMode()})
	s.persistModAppStateBestEffort()
	writeJSON(w, payload)
}

func (s *Server) modSkillViews() []modSkillView {
	ctl := s.ctl()
	all := ctl.AllSkills()
	out := make([]modSkillView, 0, len(all))
	for _, sk := range all {
		out = append(out, modSkillView{Name: sk.Name, Description: sk.Description, Scope: string(sk.Scope), RunAs: string(sk.RunAs), ReadOnly: sk.ReadOnly, Enabled: ctl.SkillEnabled(sk.Name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (s *Server) modAgentSkillsGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"skills": s.modSkillViews()})
}
func (s *Server) modAgentSkillsSet(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot change skills while a turn is running", http.StatusConflict)
		return
	}
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		http.Error(w, "invalid skill request", http.StatusBadRequest)
		return
	}
	if err := s.ctl().SetSkillEnabled(body.Name, body.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.modHub.Emit("agent.skill.updated", map[string]any{"name": body.Name, "enabled": body.Enabled, "takesEffect": "next-controller-rebuild"})
	writeJSON(w, map[string]any{"ok": true, "name": body.Name, "enabled": body.Enabled, "takesEffect": "next-controller-rebuild"})
}

type modInstructionView struct {
	Path  string `json:"path"`
	Scope string `json:"scope"`
	Body  string `json:"body,omitempty"`
}

func (s *Server) modInstructionViews(includeBody bool) []modInstructionView {
	mem := s.ctl().Memory()
	if mem == nil {
		return nil
	}
	out := make([]modInstructionView, 0, len(mem.Docs))
	for _, d := range mem.Docs {
		v := modInstructionView{Path: d.Path, Scope: string(d.Scope)}
		if includeBody {
			v.Body = d.Body
		}
		out = append(out, v)
	}
	return out
}
func (s *Server) modInstructionsGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"documents": s.modInstructionViews(true)})
}
func (s *Server) modInstructionsSet(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot edit instructions while a turn is running", http.StatusConflict)
		return
	}
	var body struct {
		Path string `json:"path"`
		Body string `json:"body"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "invalid instruction request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		mem := s.ctl().Memory()
		if mem == nil {
			http.Error(w, "memory/instructions unavailable", http.StatusConflict)
			return
		}
		body.Path = mem.DocPath(memory.ScopeProject)
	}
	written, err := s.ctl().SaveDoc(body.Path, body.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.modHub.Emit("agent.instructions.updated", map[string]any{"path": written, "bytes": len(body.Body)})
	writeJSON(w, map[string]any{"ok": true, "path": written, "applies": "next-turn-tail-and-next-session-prefix"})
}

func (s *Server) modWorkspaceGet(w http.ResponseWriter, _ *http.Request) {
	root := s.modWorkspacePath()
	writeJSON(w, map[string]any{"root": root, "switchContract": "APK supervisor restarts reasonix serve with selected directory as cwd", "liveSwitch": false})
}
func (s *Server) modWorkspaceValidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	p, err := filepath.Abs(strings.TrimSpace(body.Path))
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		http.Error(w, "workspace does not exist", http.StatusBadRequest)
		return
	}
	st, err := os.Stat(real)
	if err != nil || !st.IsDir() {
		http.Error(w, "workspace is not a directory", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"valid": true, "path": real, "restartRequired": real != s.modWorkspacePath()})
}

func (s *Server) resolveModWorkspaceExisting(rel string) (string, error) {
	root, err := filepath.Abs(s.modWorkspacePath())
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "." {
		rel = ""
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	cand := filepath.Join(root, rel)
	real, err := filepath.EvalSymlinks(cand)
	if err != nil {
		return "", err
	}
	if real != root && !strings.HasPrefix(real, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("path outside workspace")
	}
	return real, nil
}
func (s *Server) modWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	p, err := s.resolveModWorkspaceExisting(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	}
	ents, err := os.ReadDir(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	type item struct {
		Name string `json:"name"`
		Dir  bool   `json:"dir"`
		Size int64  `json:"size,omitempty"`
	}
	out := []item{}
	for i, e := range ents {
		if i >= 300 {
			break
		}
		v := item{Name: e.Name(), Dir: e.IsDir()}
		if info, x := e.Info(); x == nil {
			v.Size = info.Size()
		}
		out = append(out, v)
	}
	writeJSON(w, map[string]any{"path": p, "entries": out})
}
func (s *Server) modWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	p, err := s.resolveModWorkspaceExisting(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		http.Error(w, "not a file", http.StatusBadRequest)
		return
	}
	if st.Size() > 2<<20 {
		http.Error(w, "file too large for APK text preview", http.StatusRequestEntityTooLarge)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"path": p, "size": len(b), "text": string(b)})
}
