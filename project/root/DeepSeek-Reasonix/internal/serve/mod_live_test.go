package serve

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/efficiency"
	"reasonix/internal/event"
	"reasonix/internal/tool"
)

func newModLiveTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(modAgentTestTool{name: "read_x", ro: true})
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, Registry: reg, WorkspaceRoot: t.TempDir()})
	srv := New(ctrl, bc, config.ServeConfig{})
	return srv, httptest.NewServer(srv.Handler())
}

func TestModLiveProtocolExposesActionsButNeverReasoning(t *testing.T) {
	srv, ts := newModLiveTestServer(t)
	defer ts.Close()
	srv.observeCoreEvent(event.Event{Kind: event.Reasoning, Text: "HIDDEN_CHAIN_123"})
	srv.observeCoreEvent(event.Event{Kind: event.Text, Text: "visible answer"})
	srv.observeCoreEvent(event.Event{Kind: event.Message, Text: "visible answer", Reasoning: "HIDDEN_MESSAGE_REASONING"})
	srv.observeCoreEvent(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "1", Name: "edit_file", Args: `{"path":"main.go","content":"x"}`, ReadOnly: false, FileDiff: event.FileDiff{Diff: "--- a/main.go\n+++ b/main.go\n+x\n", Added: 1}}})
	srv.observeCoreEvent(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "1", Name: "edit_file", Output: "done sk-proj-abcdefghijklmnop", DurationMs: 5}})

	resp, err := ts.Client().Get(ts.URL + "/mod/live/history?limit=100")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if strings.Contains(text, "HIDDEN_CHAIN_123") || strings.Contains(text, "HIDDEN_MESSAGE_REASONING") {
		t.Fatalf("hidden reasoning leaked: %s", text)
	}
	for _, want := range []string{"live.chat.message", "visible answer", "live.tool.started", "live.tool.finished", `"added":1`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "sk-proj-abcdefghijklmnop") {
		t.Fatalf("credential was not redacted: %s", text)
	}
}

func TestModLiveMetadataModeOmitsProjectPayloads(t *testing.T) {
	srv, ts := newModLiveTestServer(t)
	defer ts.Close()
	if _, err := srv.modProfile.Configure(efficiency.ProjectProfile{Mode: efficiency.AgentModeAgent, LiveDetail: efficiency.LiveDetailMetadata}); err != nil {
		t.Fatal(err)
	}
	srv.observeCoreEvent(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "1", Name: "edit_file", Args: `{"secret":"SOURCE_BODY"}`, FileDiff: event.FileDiff{Diff: "DIFF_BODY", Added: 2}}})
	srv.observeCoreEvent(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "1", Name: "edit_file", Output: "OUTPUT_BODY"}})
	srv.observeCoreEvent(event.Event{Kind: event.ApprovalRequest, Approval: event.Approval{ID: "a1", Tool: "edit_file", Subject: "APPROVAL_SECRET_BODY"}})
	srv.observeCoreEvent(event.Event{Kind: event.WorkspaceChanged, Workspace: &event.WorkspaceChangedPayload{Changes: []event.WorkspacePathChange{{Path: "SECRET_FILE_NAME", Op: "write"}}}})
	resp, err := ts.Client().Get(ts.URL + "/mod/live/history")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	text := string(raw)
	for _, forbidden := range []string{"SOURCE_BODY", "DIFF_BODY", "OUTPUT_BODY", "APPROVAL_SECRET_BODY", "SECRET_FILE_NAME", "argsPreview", "outputPreview"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("metadata mode leaked %q: %s", forbidden, text)
		}
	}
}
