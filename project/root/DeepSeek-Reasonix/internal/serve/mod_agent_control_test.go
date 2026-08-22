package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/memory"
	"reasonix/internal/tool"
)

type modAgentTestTool struct {
	name string
	ro   bool
}

func (t modAgentTestTool) Name() string                                             { return t.name }
func (t modAgentTestTool) Description() string                                      { return "APK test tool" }
func (t modAgentTestTool) Schema() json.RawMessage                                  { return json.RawMessage(`{"type":"object"}`) }
func (t modAgentTestTool) Execute(context.Context, json.RawMessage) (string, error) { return "ok", nil }
func (t modAgentTestTool) ReadOnly() bool                                           { return t.ro }

func newModAgentTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("OLD RULE\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello apk\n"), 0644); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	reg.Add(modAgentTestTool{name: "read_x", ro: true})
	reg.Add(modAgentTestTool{name: "write_x"})
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, Registry: reg, WorkspaceRoot: root, Memory: memory.Load(memory.Options{CWD: root, UserDir: ""})})
	return httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler()), root
}

func TestModAgentToolsAPIUsesNativeRuntimePolicy(t *testing.T) {
	ts, _ := newModAgentTestServer(t)
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/mod/agent/tools", "application/json", strings.NewReader(`{"decisions":{"READ_X":"DENY","write_x":"allow"},"approvalMode":"ask"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, b)
	}
	got, err := http.Get(ts.URL + "/mod/agent/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	var body struct {
		Tools []modToolView `json:"tools"`
	}
	if err := json.NewDecoder(got.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	seen := map[string]modToolView{}
	for _, v := range body.Tools {
		seen[v.Name] = v
	}
	if seen["read_x"].Decision != "deny" || seen["read_x"].ProviderVisible {
		t.Fatalf("read_x=%+v", seen["read_x"])
	}
	if seen["write_x"].Decision != "allow" {
		t.Fatalf("write_x=%+v", seen["write_x"])
	}
}

func TestModInstructionsAndWorkspaceFileAPI(t *testing.T) {
	ts, root := newModAgentTestServer(t)
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/mod/instructions", "application/json", strings.NewReader(`{"path":"`+strings.ReplaceAll(filepath.Join(root, "AGENTS.md"), `\`, `\\`)+`","body":"NEW RULE\\n"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("instruction status=%d %s", resp.StatusCode, b)
	}
	raw, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "NEW RULE") {
		t.Fatalf("AGENTS.md=%q", raw)
	}
	got, err := http.Get(ts.URL + "/mod/workspace/file?path=hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	var fileBody map[string]any
	if err := json.NewDecoder(got.Body).Decode(&fileBody); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fileBody["text"].(string), "hello apk") {
		t.Fatalf("file=%v", fileBody)
	}
}

func TestModWorkspaceFileRejectsTraversal(t *testing.T) {
	ts, _ := newModAgentTestServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/mod/workspace/file?path=../secret.txt")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d want 403", resp.StatusCode)
	}
}
