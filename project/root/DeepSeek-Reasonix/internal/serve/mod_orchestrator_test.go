package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/efficiency"
	"reasonix/internal/event"
)

func TestModAutoOrchestratorLeaseAndLimit(t *testing.T) {
	o := newModAutoOrchestrator()
	if _, err := o.Configure(modAutoConfig{Enabled: true, MaxContinuations: 2, IdleWaitSeconds: 1}); err != nil {
		t.Fatal(err)
	}
	ctx, seq, ok, reason := o.tryBegin()
	if !ok || ctx == nil || seq == 0 || reason != "" {
		t.Fatalf("begin=(%v,%d,%v,%q)", ctx, seq, ok, reason)
	}
	if !o.BlocksSubmit() {
		t.Fatal("active transition must gate direct submit")
	}
	o.finish(seq, "running", string(efficiency.RouteRetryFlash), "", "q1", true)
	if o.Snapshot().Continuations != 1 || o.BlocksSubmit() {
		t.Fatalf("after first=%+v", o.Snapshot())
	}
	_, seq2, ok, _ := o.tryBegin()
	if !ok {
		t.Fatal("second continuation should be admitted")
	}
	o.finish(seq2, "running", string(efficiency.RouteDiagnosePro), "", "q2", true)
	if _, _, ok, _ := o.tryBegin(); ok {
		t.Fatal("third continuation must be blocked by maxContinuations")
	}
}

func TestModAutoContinuationPromptsCoverOnlyNonTerminalRoutes(t *testing.T) {
	for _, action := range []efficiency.RouteAction{
		efficiency.RouteRetryFlash,
		efficiency.RouteDiagnosePro,
		efficiency.RouteExecuteFlash,
		efficiency.RouteContinueFlash,
	} {
		display, prompt, ok := modAutoContinuationText(action)
		if !ok || strings.TrimSpace(display) == "" || strings.TrimSpace(prompt) == "" {
			t.Fatalf("action %s has no continuation", action)
		}
	}
	for _, action := range []efficiency.RouteAction{efficiency.RouteFinalize, efficiency.RouteStopBudget, efficiency.RouteStopNoProgress} {
		if _, _, ok := modAutoContinuationText(action); ok {
			t.Fatalf("terminal action %s must not auto-continue", action)
		}
	}
}

func TestModAutoSubmitGateRejectsDirectHTTPDuringTransition(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, ModelRef: "mock/flash", SessionDir: t.TempDir()})
	s := New(ctrl, bc, config.ServeConfig{})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	_, seq, ok, _ := s.modAuto.tryBegin()
	if !ok {
		t.Fatal("failed to acquire test continuation lease")
	}
	defer s.modAuto.finish(seq, "stopped", "", "test cleanup", "", false)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/submit", strings.NewReader(`{"input":"user turn"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d want 409", resp.StatusCode)
	}
}

func TestModAutoScheduleFailsClosedWithoutExecutionRouting(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, ModelRef: "mock/flash", SessionDir: t.TempDir()})
	s := New(ctrl, bc, config.ServeConfig{})
	s.modPowerTurn.Begin("mock/flash", "flash_primary")
	s.modPowerTurn.Observe(fakeFailedVerificationEvent())
	if _, ok := s.modPowerTurn.Finish(fakeTurnDoneEvent()); !ok {
		t.Fatal("expected pending repair attempt")
	}
	if err := s.scheduleModAutoContinuation(); err == nil || !strings.Contains(err.Error(), "execution routing is disabled") {
		t.Fatalf("err=%v", err)
	}
	if !s.modPowerTurn.Snapshot().PendingApplication {
		t.Fatal("fail-closed schedule must leave route pending")
	}
}

func fakeFailedVerificationEvent() event.Event {
	return event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "v", Name: "bash", Err: "exit 1", Execution: &event.ShellExecution{Verification: "failed"}}}
}

func fakeTurnDoneEvent() event.Event { return event.Event{Kind: event.TurnDone, Outcome: "partial"} }

func TestModAutoAPIConfigAndStop(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, ModelRef: "mock/flash", SessionDir: t.TempDir()})
	s := New(ctrl, bc, config.ServeConfig{})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mod/orchestrator/config", strings.NewReader(`{"enabled":true,"maxContinuations":3,"idleWaitSeconds":5}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config status=%d", resp.StatusCode)
	}
	if got := s.modAuto.Snapshot().Config.MaxContinuations; got != 3 {
		t.Fatalf("maxContinuations=%d", got)
	}

	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/mod/orchestrator/stop", strings.NewReader(`{}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := ts.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK || s.modAuto.Snapshot().State != "stopped" {
		t.Fatalf("stop status=%d snapshot=%+v", resp2.StatusCode, s.modAuto.Snapshot())
	}
}
