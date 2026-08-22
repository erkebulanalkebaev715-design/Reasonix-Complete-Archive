package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/efficiency"
	"reasonix/internal/tool"
)

func TestModProjectChatModeHidesMutatingToolsAndRestoresAgentMode(t *testing.T) {
	ts, _ := newModAgentTestServer(t)
	defer ts.Close()

	post := func(body string) {
		t.Helper()
		resp, err := http.Post(ts.URL+"/mod/project", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status=%d body=%s", resp.StatusCode, b)
		}
	}
	post(`{"name":"Test","mode":"chat","toolPacks":["developer"],"liveDetail":"project"}`)

	got, err := http.Get(ts.URL + "/mod/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	var body struct {
		Capabilities []modCapabilityView `json:"capabilities"`
	}
	if err := json.NewDecoder(got.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	seen := map[string]modCapabilityView{}
	for _, c := range body.Capabilities {
		seen[c.Name] = c
	}
	if !seen["read_x"].ProviderVisible || seen["read_x"].Decision == "deny" {
		t.Fatalf("read_x=%+v", seen["read_x"])
	}
	if len(seen["read_x"].Schema) == 0 {
		t.Fatal("capability registry must expose the native tool schema to the APK")
	}
	if seen["write_x"].ProviderVisible || seen["write_x"].Decision != "deny" {
		t.Fatalf("write_x=%+v", seen["write_x"])
	}

	post(`{"name":"Test","mode":"agent","toolPacks":["developer"],"liveDetail":"project"}`)
	got2, err := http.Get(ts.URL + "/mod/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer got2.Body.Close()
	body.Capabilities = nil
	if err := json.NewDecoder(got2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	seen = map[string]modCapabilityView{}
	for _, c := range body.Capabilities {
		seen[c.Name] = c
	}
	if !seen["write_x"].ProviderVisible || seen["write_x"].Decision == "deny" {
		t.Fatalf("write_x not restored in agent mode: %+v", seen["write_x"])
	}
}

func TestModProjectToolPackRestrictsSurfaceAndManualDenySurvives(t *testing.T) {
	ts, _ := newModAgentTestServer(t)
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/mod/agent/tools", "application/json", strings.NewReader(`{"decisions":{"read_x":"deny"}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tool policy status=%d", resp.StatusCode)
	}
	resp, err = http.Post(ts.URL+"/mod/project", "application/json", strings.NewReader(`{"mode":"agent","toolPacks":["basic"],"liveDetail":"metadata"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("project status=%d", resp.StatusCode)
	}
	got, err := http.Get(ts.URL + "/mod/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	raw, _ := io.ReadAll(got.Body)
	if !strings.Contains(string(raw), `"manualDecision":"deny"`) {
		t.Fatalf("manual deny not preserved: %s", raw)
	}
	var caps struct {
		Capabilities []modCapabilityView `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &caps); err != nil {
		t.Fatal(err)
	}
	seen := map[string]modCapabilityView{}
	for _, c := range caps.Capabilities {
		seen[c.Name] = c
	}
	if seen["write_x"].Decision != "deny" || seen["write_x"].ProviderVisible {
		t.Fatalf("basic pack must hide writer: %+v", seen["write_x"])
	}
}

func TestModEnvironmentAndProjectContract(t *testing.T) {
	ts, root := newModAgentTestServer(t)
	defer ts.Close()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module apk-test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(ts.URL + "/mod/environment")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("environment=%d %s", resp.StatusCode, raw)
	}
	var env modEnvironmentSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Source != "reasonix-native-environment" {
		t.Fatalf("environment source=%q", env.Source)
	}
	if env.Workspace != root {
		t.Fatalf("workspace=%q want %q", env.Workspace, root)
	}
	if !containsString(env.ProjectKinds, "go") || !containsString(env.Markers, "go.mod") {
		t.Fatalf("environment markers=%v kinds=%v", env.Markers, env.ProjectKinds)
	}

	project, err := http.Get(ts.URL + "/mod/project")
	if err != nil {
		t.Fatal(err)
	}
	defer project.Body.Close()
	p, _ := io.ReadAll(project.Body)
	for _, want := range []string{`"chatEndpoint":"/submit"`, `"eventsEndpoint":"/mod/events"`, `"historyEndpoint":"/mod/live/history"`} {
		if !strings.Contains(string(p), want) {
			t.Fatalf("project contract missing %s: %s", want, p)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestModProDiagnosisHardDeniesMutatingTools(t *testing.T) {
	ts, _ := newModAgentTestServer(t)
	defer ts.Close()
	// Reach into the handler server through a direct instance so the execution
	// overlay itself, not a chat/profile deny, is what hides the writer.
	root := t.TempDir()
	reg := tool.NewRegistry()
	reg.Add(modAgentTestTool{name: "read_x", ro: true})
	reg.Add(modAgentTestTool{name: "write_x"})
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, Registry: reg, WorkspaceRoot: root, ModelRef: "mock/pro"})
	s := New(ctrl, bc, config.ServeConfig{})
	s.setModExecutionPolicyMode(efficiency.ExecutionProDiagnosis)
	if err := s.applyModToolDecisionsToCurrentController(); err != nil {
		t.Fatal(err)
	}
	views := s.modToolViews()
	seen := map[string]modToolView{}
	for _, v := range views {
		seen[v.Name] = v
	}
	if seen["read_x"].Decision == "deny" || !seen["read_x"].ProviderVisible {
		t.Fatalf("read tool unexpectedly blocked: %+v", seen["read_x"])
	}
	if seen["write_x"].Decision != "deny" || seen["write_x"].ProviderVisible {
		t.Fatalf("pro diagnosis writer not hard-denied: %+v", seen["write_x"])
	}
}
