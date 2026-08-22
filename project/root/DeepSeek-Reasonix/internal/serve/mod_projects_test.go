package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

func TestModProjectRegistryAndSupervisorHandoff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	rootA := t.TempDir()
	rootB := t.TempDir()
	ts, _ := newPersistentModServer(t, rootA)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/mod/projects/register", `{"name":"Alpha","workspace":`+jsonQuote(rootA)+`}`)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register A=%d %s", resp.StatusCode, body)
	}
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &a); err != nil || a.ID == "" {
		t.Fatalf("register A response=%s err=%v", body, err)
	}

	resp = postJSON(t, ts.URL+"/mod/projects/register", `{"name":"Beta","workspace":`+jsonQuote(rootB)+`}`)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register B=%d %s", resp.StatusCode, body)
	}
	var b struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &b); err != nil || b.ID == "" || b.ID == a.ID {
		t.Fatalf("register B response=%s err=%v", body, err)
	}

	resp, err := http.Get(ts.URL + "/mod/projects")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"budgetScope":"workspace"`) || !strings.Contains(string(body), `"current":true`) {
		t.Fatalf("projects=%d %s", resp.StatusCode, body)
	}

	resp = postJSON(t, ts.URL+"/mod/projects/open", `{"id":`+jsonQuote(b.ID)+`}`)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open B=%d %s", resp.StatusCode, body)
	}
	var handoff struct {
		RestartRequired bool   `json:"restartRequired"`
		Workspace       string `json:"workspace"`
		Supervisor      struct {
			Action    string `json:"action"`
			Workspace string `json:"workspace"`
		} `json:"supervisor"`
	}
	if err := json.Unmarshal(body, &handoff); err != nil {
		t.Fatal(err)
	}
	if !handoff.RestartRequired || canonicalModWorkspace(handoff.Workspace) != canonicalModWorkspace(rootB) || handoff.Supervisor.Action != "restart" {
		t.Fatalf("handoff=%+v", handoff)
	}
	if got := canonicalModWorkspace(rootA); got != canonicalModWorkspace(rootA) {
		t.Fatal("unreachable")
	}

	resp = postJSON(t, ts.URL+"/mod/projects/remove", `{"id":`+jsonQuote(b.ID)+`}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove B=%d", resp.StatusCode)
	}
	if st, err := os.Stat(rootB); err != nil || !st.IsDir() {
		t.Fatalf("registry removal must not delete workspace: %v", err)
	}
}

func TestModTaskCatalogUsesNativeSessionsAndRename(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := t.TempDir()
	dir := filepath.Join(root, ".sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := agent.NewSessionPath(dir, "apk-task")
	sess := agent.NewSession("sys")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "Fix the offline test project"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "Working"})
	if err := sess.Save(path); err != nil {
		t.Fatal(err)
	}

	ts, _ := newPersistentModServer(t, root)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/mod/tasks")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tasks=%d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"providerCallsForListing":false`) || !strings.Contains(string(body), "Fix the offline test project") {
		t.Fatalf("tasks body=%s", body)
	}

	resp = postJSON(t, ts.URL+"/mod/tasks/rename", `{"path":`+jsonQuote(path)+`,"title":"APK task"}`)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename=%d %s", resp.StatusCode, body)
	}
	items, err := agent.ListSessions(dir)
	if err != nil || len(items) != 1 || items[0].CustomTitle != "APK task" {
		t.Fatalf("renamed sessions=%+v err=%v", items, err)
	}

	outsideDir := t.TempDir()
	outside := agent.NewSessionPath(outsideDir, "outside")
	other := agent.NewSession("sys")
	other.Add(provider.Message{Role: provider.RoleUser, Content: "outside"})
	if err := other.Save(outside); err != nil {
		t.Fatal(err)
	}
	resp = postJSON(t, ts.URL+"/mod/tasks/rename", `{"path":`+jsonQuote(outside)+`,"title":"Nope"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("outside rename=%d", resp.StatusCode)
	}
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
