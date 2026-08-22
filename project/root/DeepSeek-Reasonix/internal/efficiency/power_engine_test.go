package efficiency

import (
	"context"
	"testing"
)

func TestPowerEngineUnifiesRepairRouteBudgetAndExecution(t *testing.T) {
	gov := NewGovernor()
	gov.Configure(BudgetConfig{BudgetKZT: 100, ReservePercent: 10, ProMaxPercent: 50, HardStop: true, FXKZTPerUnit: map[string]float64{"KZT": 1}})
	esc := NewEscalator()
	cycle := NewRepairCycle(esc, gov, nil)
	exec := NewExecutionRouter(gov)
	switches := []string{}
	exec.SetSwitcher(func(_ context.Context, ref string) error {
		switches = append(switches, ref)
		return nil
	})
	if _, err := exec.Configure(ExecutionConfig{Enabled: true, FlashPrimaryRef: "flash-a", FlashAlternativeRef: "flash-b", ProRef: "pro", FlashRepairRef: "flash-a"}, "flash-a"); err != nil {
		t.Fatal(err)
	}
	p := NewPowerEngine(cycle, exec, gov)

	failed := VerificationReceipt{RequiredChecks: 1, ChecksFailed: 1}
	passed := VerificationReceipt{RequiredChecks: 1, ChecksPassed: 1}
	steps := []PowerAttempt{
		{RepairAttempt: RepairAttempt{StrategyID: "A", FailureFingerprint: "E1", Verification: failed, EstimatedFlashKZT: 1, EstimatedProKZT: 5}, CurrentModelRef: "flash-a"},
		{RepairAttempt: RepairAttempt{StrategyID: "B", FailureFingerprint: "E1", Verification: failed, EstimatedFlashKZT: 1, EstimatedProKZT: 5}, CurrentModelRef: "flash-b"},
		{RepairAttempt: RepairAttempt{StrategyID: "pro-diagnosis", FailureFingerprint: "E1", Verification: failed, EstimatedFlashKZT: 1, EstimatedProKZT: 5}, CurrentModelRef: "pro"},
		{RepairAttempt: RepairAttempt{StrategyID: "repair", ResolvedFingerprint: "E1", Verification: passed, Finalization: true}, CurrentModelRef: "flash-a"},
	}

	want := []RouteAction{RouteRetryFlash, RouteDiagnosePro, RouteExecuteFlash, RouteFinalize}
	for i, step := range steps {
		got, err := p.Handle(context.Background(), step)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if got.Snapshot.NextAction != want[i] {
			t.Fatalf("step %d action=%q want=%q snapshot=%+v", i, got.Snapshot.NextAction, want[i], got.Snapshot)
		}
	}
	if len(switches) < 3 {
		t.Fatalf("switches=%v, want flash-b -> pro -> flash-a", switches)
	}
	if got := p.Snapshot(); !got.Terminal || got.NextAction != RouteFinalize {
		t.Fatalf("final snapshot=%+v", got)
	}
}

func TestPowerEngineBudgetBlocksProBeforeSwitch(t *testing.T) {
	gov := NewGovernor()
	gov.Configure(BudgetConfig{BudgetKZT: 1, ReservePercent: 0, ProMaxPercent: 10, HardStop: true, FXKZTPerUnit: map[string]float64{"KZT": 1}})
	cycle := NewRepairCycle(NewEscalator(), gov, nil)
	exec := NewExecutionRouter(gov)
	calls := 0
	exec.SetSwitcher(func(context.Context, string) error { calls++; return nil })
	if _, err := exec.Configure(ExecutionConfig{Enabled: true, FlashPrimaryRef: "flash-a", FlashAlternativeRef: "flash-b", ProRef: "pro"}, "flash-a"); err != nil {
		t.Fatal(err)
	}
	p := NewPowerEngine(cycle, exec, gov)
	fail := VerificationReceipt{RequiredChecks: 1, ChecksFailed: 1}
	if _, err := p.Handle(context.Background(), PowerAttempt{RepairAttempt: RepairAttempt{StrategyID: "A", FailureFingerprint: "E", Verification: fail, EstimatedFlashKZT: .1, EstimatedProKZT: 5}, CurrentModelRef: "flash-a"}); err != nil {
		t.Fatal(err)
	}
	before := calls
	got, err := p.Handle(context.Background(), PowerAttempt{RepairAttempt: RepairAttempt{StrategyID: "B", FailureFingerprint: "E", Verification: fail, EstimatedFlashKZT: 2, EstimatedProKZT: 5}, CurrentModelRef: "flash-b"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.NextAction != RouteStopBudget {
		t.Fatalf("action=%q want=%q snapshot=%+v", got.Snapshot.NextAction, RouteStopBudget, got.Snapshot)
	}
	if calls != before {
		t.Fatalf("terminal budget stop must not call switcher: before=%d after=%d", before, calls)
	}
}
