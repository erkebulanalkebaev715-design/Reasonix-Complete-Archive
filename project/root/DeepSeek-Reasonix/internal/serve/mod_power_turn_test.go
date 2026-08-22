package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

func TestModPowerTurnTrackerBuildsHostVerifiedAttempt(t *testing.T) {
	tr := newModPowerTurnTracker()
	tr.Begin("mock/flash", "flash_primary")
	tr.Observe(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "w1", Name: "edit_file", ReadOnly: false, FileDiff: event.FileDiff{Added: 3, Removed: 1, Diff: "+x"}}})
	tr.Observe(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "v1", Name: "bash", Execution: &event.ShellExecution{Verification: "failed", FailurePhase: "exit"}, Err: "exit status 1"}})
	tr.Observe(event.Event{Kind: event.CompletionSummary, Completion: &event.CompletionSummaryInfo{ChecksPassed: 0, ChecksFailed: 1, Mutations: 1}})
	attempt, ok := tr.Finish(event.Event{Kind: event.TurnDone, Outcome: "partial"})
	if !ok {
		t.Fatal("expected a power attempt")
	}
	if attempt.StrategyID != "flash_primary" || attempt.Verification.RequiredChecks != 1 || attempt.Verification.ChecksFailed != 1 {
		t.Fatalf("attempt=%+v", attempt)
	}
	if attempt.FailureFingerprint == "" || attempt.Finalization {
		t.Fatalf("failure attempt=%+v", attempt)
	}
	if attempt.PatchNumstat != "3\t1\t<turn>" {
		t.Fatalf("patch=%q", attempt.PatchNumstat)
	}
}

func TestModPowerTurnTrackerRefusesUnverifiedMutationAsDone(t *testing.T) {
	tr := newModPowerTurnTracker()
	tr.Begin("mock/flash", "flash_primary")
	tr.Observe(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "w1", Name: "edit_file", ReadOnly: false, FileDiff: event.FileDiff{Added: 1, Diff: "+x"}}})
	attempt, ok := tr.Finish(event.Event{Kind: event.TurnDone})
	if !ok {
		t.Fatal("expected mutating turn to be evaluated")
	}
	if attempt.Verification.ChecksFailed != 1 || attempt.Finalization {
		t.Fatalf("unverified mutation was treated as done: %+v", attempt)
	}
}

func TestModPowerAPIExposesUnifiedState(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, ModelRef: "mock/flash"})
	srv := New(ctrl, bc, config.ServeConfig{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/mod/power")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /mod/power = %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["engine"] == nil || body["turn"] == nil {
		t.Fatalf("body=%#v", body)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mod/power/reset", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST /mod/power/reset = %d", resp2.StatusCode)
	}
}

func TestModPowerPendingRouteAppliesOnlyAtExplicitIdleBoundary(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, ModelRef: "mock/flash"})
	srv := New(ctrl, bc, config.ServeConfig{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	srv.modPowerTurn.Begin("mock/flash", "flash_primary")
	srv.modPowerTurn.Observe(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "w", Name: "edit_file", ReadOnly: false, FileDiff: event.FileDiff{Added: 1, Diff: "+x"}}})
	srv.modPowerTurn.Observe(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "v", Name: "bash", Err: "exit 1", Execution: &event.ShellExecution{Verification: "failed"}}})
	if _, ok := srv.modPowerTurn.Finish(event.Event{Kind: event.TurnDone, Outcome: "partial"}); !ok {
		t.Fatal("expected pending attempt")
	}
	if !srv.modPowerTurn.Snapshot().PendingApplication {
		t.Fatal("route should stay pending until explicit idle-boundary apply")
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mod/power/apply-pending", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply pending = %d", resp.StatusCode)
	}
	if srv.modPowerTurn.Snapshot().PendingApplication {
		t.Fatal("successful apply must clear pending route")
	}
}
