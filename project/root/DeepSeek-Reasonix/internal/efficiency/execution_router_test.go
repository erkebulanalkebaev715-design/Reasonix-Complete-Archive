package efficiency

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func testExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		Enabled:             true,
		FlashPrimaryRef:     "mock/flash-a",
		FlashAlternativeRef: "mock/flash-b",
		ProRef:              "mock/pro",
		FlashRepairRef:      "mock/flash-repair",
	}
}

func TestExecutionRouterMapsFlashFlashProFlash(t *testing.T) {
	g := NewGovernor()
	g.Configure(BudgetConfig{BudgetKZT: 1000, ReservePercent: 10, ProMaxPercent: 50, HardStop: true})
	r := NewExecutionRouter(g)
	var switched []string
	r.SetSwitcher(func(_ context.Context, ref string) error {
		switched = append(switched, ref)
		return nil
	})
	if _, err := r.Configure(testExecutionConfig(), "mock/flash-a"); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		decision RouteDecision
		current  string
		wantMode ExecutionMode
		wantRef  string
	}{
		{RouteDecision{Action: RouteRetryFlash, FlashAttempts: 1}, "mock/flash-a", ExecutionFlashAlternative, "mock/flash-b"},
		{RouteDecision{Action: RouteDiagnosePro, FlashAttempts: 1}, "mock/flash-b", ExecutionProDiagnosis, "mock/pro"},
		{RouteDecision{Action: RouteExecuteFlash, FlashAttempts: 2}, "mock/pro", ExecutionFlashRepair, "mock/flash-repair"},
		{RouteDecision{Action: RouteFinalize}, "mock/flash-repair", ExecutionFinished, ""},
	}
	for _, step := range steps {
		snap, err := r.Apply(context.Background(), ExecutionRequest{
			Decision: step.decision, CurrentModelRef: step.current,
			EstimatedFlashKZT: 2, EstimatedProKZT: 5, Finalization: step.decision.Action == RouteFinalize,
		})
		if err != nil {
			t.Fatalf("%s: %v", step.decision.Action, err)
		}
		if snap.Mode != step.wantMode || snap.TargetModelRef != step.wantRef {
			t.Fatalf("%s snapshot=%+v", step.decision.Action, snap)
		}
		if step.wantMode == ExecutionProDiagnosis && !snap.DiagnosisOnly {
			t.Fatal("Pro phase must be marked diagnosis-only")
		}
	}
	want := []string{"mock/flash-b", "mock/pro", "mock/flash-repair"}
	if !reflect.DeepEqual(switched, want) {
		t.Fatalf("switches=%v want=%v", switched, want)
	}
}

func TestExecutionRouterBudgetRechecksBeforeSwitch(t *testing.T) {
	g := NewGovernor()
	g.Configure(BudgetConfig{BudgetKZT: 100, ReservePercent: 0, ProMaxPercent: 10, HardStop: true})
	r := NewExecutionRouter(g)
	called := false
	r.SetSwitcher(func(context.Context, string) error { called = true; return nil })
	if _, err := r.Configure(testExecutionConfig(), "mock/flash-b"); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Apply(context.Background(), ExecutionRequest{
		Decision: RouteDecision{Action: RouteDiagnosePro}, CurrentModelRef: "mock/flash-b", EstimatedProKZT: 11,
	})
	if err == nil || !snap.Blocked || called {
		t.Fatalf("snap=%+v err=%v called=%v", snap, err, called)
	}
}

func TestExecutionRouterSwitchFailureIsFailClosed(t *testing.T) {
	r := NewExecutionRouter(nil)
	r.SetSwitcher(func(context.Context, string) error { return errors.New("boom") })
	if _, err := r.Configure(testExecutionConfig(), "mock/flash-a"); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Apply(context.Background(), ExecutionRequest{
		Decision: RouteDecision{Action: RouteRetryFlash, FlashAttempts: 1}, CurrentModelRef: "mock/flash-a",
	})
	if err == nil || !snap.Blocked || snap.CurrentModelRef != "mock/flash-a" {
		t.Fatalf("snap=%+v err=%v", snap, err)
	}
}

func TestExecutionRouterNoSwitcherNeededWhenRefAlreadyActive(t *testing.T) {
	r := NewExecutionRouter(nil)
	cfg := testExecutionConfig()
	cfg.FlashAlternativeRef = cfg.FlashPrimaryRef
	if _, err := r.Configure(cfg, cfg.FlashPrimaryRef); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Apply(context.Background(), ExecutionRequest{
		Decision: RouteDecision{Action: RouteRetryFlash, FlashAttempts: 1}, CurrentModelRef: cfg.FlashPrimaryRef,
	})
	if err != nil || snap.Blocked || snap.CurrentModelRef != cfg.FlashPrimaryRef {
		t.Fatalf("snap=%+v err=%v", snap, err)
	}
}

func TestExecutionRouterModeGuardRunsBeforeSwitchAndRollsBackOnFailure(t *testing.T) {
	gov := NewGovernor()
	r := NewExecutionRouter(gov)
	if _, err := r.Configure(ExecutionConfig{Enabled: true, FlashPrimaryRef: "flash", ProRef: "pro"}, "flash"); err != nil {
		t.Fatal(err)
	}
	var guarded ExecutionMode
	var undone bool
	r.SetModeGuard(func(_ context.Context, mode ExecutionMode, _ string) (func(), error) {
		guarded = mode
		return func() { undone = true }, nil
	})
	r.SetSwitcher(func(context.Context, string) error { return fmt.Errorf("switch failed") })
	_, err := r.Apply(context.Background(), ExecutionRequest{Decision: RouteDecision{Action: RouteDiagnosePro}, CurrentModelRef: "flash"})
	if err == nil {
		t.Fatal("expected switch failure")
	}
	if guarded != ExecutionProDiagnosis || !undone {
		t.Fatalf("guarded=%q undone=%v", guarded, undone)
	}
}
