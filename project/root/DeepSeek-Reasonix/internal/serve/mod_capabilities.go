package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/efficiency"
	"reasonix/internal/environment"
	"reasonix/internal/tool"
)

type modCapabilityView struct {
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	ReadOnly        bool            `json:"readOnly"`
	ProviderVisible bool            `json:"providerVisible"`
	Decision        string          `json:"decision"`
	ManualDecision  string          `json:"manualDecision"`
	Packs           []string        `json:"packs"`
	Schema          json.RawMessage `json:"schema,omitempty"`
}

type modToolPackView struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
}

func toolPackNamesFor(e tool.ContractEntry) []string {
	name := strings.ToLower(strings.TrimSpace(e.Name))
	desc := strings.ToLower(e.Description)
	text := name + " " + desc
	packs := map[string]bool{"developer": true}
	if e.ReadOnly || name == "use_capability" {
		packs["basic"] = true
	}
	if strings.Contains(text, "file") || strings.Contains(text, "read") || strings.Contains(text, "write") || strings.Contains(text, "edit") || strings.Contains(text, "grep") || strings.Contains(text, "search") || strings.Contains(text, "list") || strings.Contains(text, "find") {
		packs["files"] = true
	}
	if strings.Contains(text, "test") || strings.Contains(text, "lint") || strings.Contains(text, "check") || strings.Contains(text, "build") || strings.Contains(text, "doctor") || strings.Contains(text, "verify") {
		packs["verify"] = true
	}
	if strings.Contains(text, "git") || strings.Contains(text, "version control") || strings.Contains(text, "diff") {
		packs["vcs"] = true
	}
	if strings.Contains(text, "bash") || strings.Contains(text, "shell") || strings.Contains(text, "terminal") || strings.Contains(text, "command") {
		packs["shell"] = true
	}
	if name == "use_capability" {
		// Keep the stable proxy available in selected packs. The resolved target
		// still passes through Reasonix's native per-tool permission gate, so a
		// hidden/denied writer cannot be reached through the proxy.
		packs["files"] = true
		packs["verify"] = true
		packs["vcs"] = true
		packs["shell"] = true
	}
	out := make([]string, 0, len(packs))
	for p := range packs {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (s *Server) modToolPacks() []modToolPackView {
	ctl, ok := s.ctl().(modToolPolicyController)
	if !ok {
		return nil
	}
	byPack := map[string][]string{
		"basic": {}, "files": {}, "verify": {}, "vcs": {}, "shell": {}, "developer": {},
	}
	for _, e := range ctl.AllToolContractEntries() {
		for _, p := range toolPackNamesFor(e) {
			byPack[p] = append(byPack[p], e.Name)
		}
	}
	desc := map[string]string{
		"basic":     "Read-only inspection and capability discovery",
		"files":     "Workspace file/search operations",
		"verify":    "Build, test, lint and verification operations",
		"vcs":       "Version-control and diff operations",
		"shell":     "Shell/command execution operations",
		"developer": "All registered development tools",
	}
	order := []string{"basic", "files", "verify", "vcs", "shell", "developer"}
	out := make([]modToolPackView, 0, len(order))
	for _, name := range order {
		tools := append([]string(nil), byPack[name]...)
		sort.Strings(tools)
		out = append(out, modToolPackView{Name: name, Description: desc[name], Tools: tools})
	}
	return out
}

func (s *Server) knownModToolPack(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range s.modToolPacks() {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (s *Server) modEffectiveToolDecisions() map[string]string {
	manual := s.modToolDecisionsSnapshot()
	effective := make(map[string]string, len(manual))
	for k, v := range manual {
		effective[k] = v
	}
	ctl, ok := s.ctl().(modToolPolicyController)
	if !ok {
		return effective
	}
	profile := s.modProfile.Snapshot()
	diagnosisReadOnly := s.modExecutionPolicyModeSnapshot() == efficiency.ExecutionProDiagnosis
	allowedByPack := map[string]bool{}
	if len(profile.ToolPacks) > 0 {
		selected := map[string]bool{}
		for _, p := range profile.ToolPacks {
			selected[strings.ToLower(strings.TrimSpace(p))] = true
		}
		for _, e := range ctl.AllToolContractEntries() {
			for _, p := range toolPackNamesFor(e) {
				if selected[p] {
					allowedByPack[e.Name] = true
					break
				}
			}
		}
	}
	for _, e := range ctl.AllToolContractEntries() {
		if diagnosisReadOnly && !e.ReadOnly && e.Name != "use_capability" {
			effective[e.Name] = "deny"
			continue
		}
		if len(profile.ToolPacks) > 0 && !allowedByPack[e.Name] {
			effective[e.Name] = "deny"
			continue
		}
		if profile.Mode == efficiency.AgentModeChat && !e.ReadOnly && e.Name != "use_capability" {
			effective[e.Name] = "deny"
		}
	}
	return effective
}

type modEnvironmentProbe struct {
	Command string `json:"command"`
	Binary  string `json:"binary"`
	Output  string `json:"output,omitempty"`
	Found   bool   `json:"found"`
	Error   string `json:"error,omitempty"`
}

type modEnvironmentSnapshot struct {
	Enabled        bool                  `json:"enabled"`
	ModelFacing    bool                  `json:"modelFacing"`
	Source         string                `json:"source"`
	OS             string                `json:"os"`
	Arch           string                `json:"arch"`
	Workspace      string                `json:"workspace"`
	ProjectKinds   []string              `json:"projectKinds,omitempty"`
	Markers        []string              `json:"markers,omitempty"`
	Probes         []modEnvironmentProbe `json:"probes,omitempty"`
	DiscoveredAtMS int64                 `json:"discoveredAtMs"`
}

func modProjectMarkers(root string) ([]string, []string) {
	markers := []struct {
		path string
		kind string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"pyproject.toml", "python"},
		{"requirements.txt", "python"},
		{"Cargo.toml", "rust"},
		{"build.gradle", "gradle"},
		{"build.gradle.kts", "gradle"},
		{"settings.gradle", "gradle"},
		{"settings.gradle.kts", "gradle"},
		{filepath.Join("app", "build.gradle"), "android"},
		{filepath.Join("app", "build.gradle.kts"), "android"},
		{filepath.Join("app", "src", "main", "AndroidManifest.xml"), "android"},
		{".git", "git"},
	}
	kinds := map[string]struct{}{}
	found := map[string]struct{}{}
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(root, marker.path)); err == nil {
			kinds[marker.kind] = struct{}{}
			found[filepath.ToSlash(marker.path)] = struct{}{}
		}
	}
	outKinds := make([]string, 0, len(kinds))
	for kind := range kinds {
		outKinds = append(outKinds, kind)
	}
	outMarkers := make([]string, 0, len(found))
	for marker := range found {
		outMarkers = append(outMarkers, marker)
	}
	sort.Strings(outKinds)
	sort.Strings(outMarkers)
	return outKinds, outMarkers
}

// discoverModEnvironment is only an APK projection. It deliberately reuses
// Reasonix's native environment probe/cache/snapshot implementation — the same
// mechanism boot uses for the model-facing cache-stable Environment section.
// Project markers are cheap APK metadata and never replace the native probes.
func discoverModEnvironment(ctx context.Context, root string, runProbes bool) modEnvironmentSnapshot {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	kinds, markers := modProjectMarkers(root)
	snap := modEnvironmentSnapshot{
		Source:         "reasonix-native-environment",
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Workspace:      root,
		ProjectKinds:   kinds,
		Markers:        markers,
		DiscoveredAtMS: time.Now().UnixMilli(),
	}
	cfg, err := config.LoadForRootReadOnly(root)
	if err != nil {
		// Fail closed for host execution metadata rather than inventing a probe
		// policy when trusted/project configuration could not be resolved.
		return snap
	}
	snap.Enabled = cfg.EnvironmentEnabled()
	snap.ModelFacing = snap.Enabled
	if !runProbes || !snap.Enabled {
		return snap
	}
	results := environment.RunProbesWithOptions(ctx, environment.DefaultProbes(), environment.ProbeOptions{
		Overrides:   cfg.Environment.Tools,
		DenyRoots:   []string{root},
		SnapshotDir: config.CacheDir(),
	})
	snap.Probes = make([]modEnvironmentProbe, 0, len(results))
	for _, result := range results {
		snap.Probes = append(snap.Probes, modEnvironmentProbe{
			Command: result.Command,
			Binary:  result.Binary,
			Output:  result.Output,
			Found:   result.Found,
			Error:   result.Error,
		})
	}
	return snap
}

func (s *Server) modEnvironmentSnapshot(ctx context.Context, refresh bool) modEnvironmentSnapshot {
	if refresh {
		snap := discoverModEnvironment(ctx, s.modWorkspacePath(), true)
		s.modMu.Lock()
		s.modEnv = snap
		s.modMu.Unlock()
		return snap
	}
	s.modMu.RLock()
	snap := s.modEnv
	s.modMu.RUnlock()
	if strings.TrimSpace(snap.Workspace) == "" {
		snap = discoverModEnvironment(ctx, s.modWorkspacePath(), false)
		s.modMu.Lock()
		s.modEnv = snap
		s.modMu.Unlock()
	}
	return snap
}

func (s *Server) modCapabilitiesGet(w http.ResponseWriter, _ *http.Request) {
	ctl, ok := s.ctl().(modToolPolicyController)
	if !ok {
		http.Error(w, "runtime capability registry unavailable", http.StatusNotImplemented)
		return
	}
	visible := map[string]bool{}
	for _, e := range ctl.ToolContractEntries() {
		visible[e.Name] = true
	}
	effective := ctl.SessionToolDecisions()
	manual := s.modToolDecisionsSnapshot()
	caps := make([]modCapabilityView, 0)
	for _, e := range ctl.AllToolContractEntries() {
		d := effective[e.Name]
		if d == "" {
			d = "inherit"
		}
		md := manual[e.Name]
		if md == "" {
			md = "inherit"
		}
		caps = append(caps, modCapabilityView{
			Name: e.Name, Description: e.Description, ReadOnly: e.ReadOnly,
			ProviderVisible: visible[e.Name], Decision: d, ManualDecision: md,
			Packs: toolPackNamesFor(e), Schema: append(json.RawMessage(nil), e.Schema...),
		})
	}
	sort.Slice(caps, func(i, j int) bool { return caps[i].Name < caps[j].Name })
	writeJSON(w, map[string]any{"profile": s.modProfile.Snapshot(), "packs": s.modToolPacks(), "capabilities": caps})
}

func (s *Server) modEnvironmentGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.modEnvironmentSnapshot(r.Context(), true))
}

func (s *Server) modProjectGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"profile":            s.modProfile.Snapshot(),
		"workspace":          s.modWorkspacePath(),
		"environment":        s.modEnvironmentSnapshot(r.Context(), false),
		"chatEndpoint":       "/submit",
		"eventsEndpoint":     "/mod/events",
		"historyEndpoint":    "/mod/live/history",
		"profilePersistence": "backend-persisted per workspace; APK may re-apply atomically",
		"reasoningPolicy":    "hidden-chain-never-exported; use live plans/actions/results",
	})
}

func (s *Server) modProjectSet(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot change project profile while a turn is running", http.StatusConflict)
		return
	}
	var p efficiency.ProjectProfile
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		http.Error(w, "invalid project profile: "+err.Error(), http.StatusBadRequest)
		return
	}
	for _, pack := range p.ToolPacks {
		if !s.knownModToolPack(pack) {
			http.Error(w, fmt.Sprintf("unknown tool pack: %s", strings.TrimSpace(pack)), http.StatusBadRequest)
			return
		}
	}
	before := s.modProfile.Snapshot()
	snap, err := s.modProfile.Configure(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.applyModToolDecisionsToCurrentController(); err != nil {
		_, _ = s.modProfile.Configure(before)
		_ = s.applyModToolDecisionsToCurrentController()
		http.Error(w, "apply project profile: "+err.Error(), http.StatusConflict)
		return
	}
	s.modHub.Emit("project.profile.updated", map[string]any{"profile": snap, "workspace": s.modWorkspacePath()})
	s.persistModAppStateBestEffort()
	writeJSON(w, map[string]any{"profile": snap, "tools": s.modToolViews(), "environment": s.modEnvironmentSnapshot(r.Context(), false)})
}
