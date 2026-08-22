package efficiency

import (
	"errors"
	"path/filepath"
	"testing"
)

func failedReceipt() VerificationReceipt {
	return VerificationReceipt{RequiredChecks: 2, ChecksPassed: 1, ChecksFailed: 1, BuildObserved: true, BuildExitCode: 1}
}

func passedReceipt() VerificationReceipt {
	return VerificationReceipt{RequiredChecks: 2, ChecksPassed: 2, BuildObserved: true, BuildExitCode: 0}
}

func TestVerificationReceiptRequiresHostEvidence(t *testing.T) {
	if (VerificationReceipt{}).Passed() {
		t.Fatal("empty/model-only completion must not pass")
	}
	if (VerificationReceipt{RequiredChecks: 1, ChecksPassed: 1, BuildObserved: true, BuildExitCode: 1}).Passed() {
		t.Fatal("failed observed build must not pass")
	}
	if !passedReceipt().Passed() {
		t.Fatal("complete host evidence should pass")
	}
}

func TestRepairCycleFlashFlashProFlashAndVerifiedCache(t *testing.T) {
	cache, err := OpenFailureCache(filepath.Join(t.TempDir(), "failures.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	g := NewGovernor()
	g.Configure(BudgetConfig{BudgetKZT: 1000, ReservePercent: 15, ProMaxPercent: 25, HardStop: true, FXKZTPerUnit: map[string]float64{"KZT": 1}})
	cycle := NewRepairCycle(NewEscalator(), g, cache)

	a, err := cycle.Report(RepairAttempt{
		StrategyID: "A", FailureFingerprint: "E1", Environment: "go1.26/linux-arm64",
		Files: []string{"main.go"}, PatchNumstat: "2\t1\tmain.go", Verification: failedReceipt(), EstimatedFlashKZT: 1, EstimatedProKZT: 5,
		BuildLog: "compile\nmain.go:4: error: bad symbol\nFAILED build",
	})
	if err != nil || a.Snapshot.LastRoute.Action != RouteRetryFlash {
		t.Fatalf("A=%+v err=%v", a.Snapshot, err)
	}
	if a.LogSummary.SelectedLines == 0 || len(a.LogSummary.Text) == 0 {
		t.Fatal("failed build log was not reduced for the next model call")
	}

	b, err := cycle.Report(RepairAttempt{
		StrategyID: "B", FailureFingerprint: "E1", Environment: "go1.26/linux-arm64",
		Files: []string{"main.go"}, PatchNumstat: "1\t1\tmain.go", Verification: failedReceipt(), EstimatedFlashKZT: 1, EstimatedProKZT: 5,
	})
	if err != nil || b.Snapshot.LastRoute.Action != RouteDiagnosePro {
		t.Fatalf("B=%+v err=%v", b.Snapshot, err)
	}

	p, err := cycle.Report(RepairAttempt{
		StrategyID: "pro-diagnosis", FailureFingerprint: "E1", Verification: failedReceipt(), EstimatedFlashKZT: 1,
	})
	if err != nil || p.Snapshot.LastRoute.Action != RouteExecuteFlash {
		t.Fatalf("pro=%+v err=%v", p.Snapshot, err)
	}

	pass, err := cycle.Report(RepairAttempt{
		StrategyID: "C", ResolvedFingerprint: "E1", Environment: "go1.26/linux-arm64", Files: []string{"main.go"},
		FixHint: "repair the symbol import", PatchNumstat: "1\t0\tmain.go", Verification: passedReceipt(), Finalization: true,
	})
	if err != nil || pass.Snapshot.LastRoute.Action != RouteFinalize || pass.Snapshot.State != "done" {
		t.Fatalf("pass=%+v err=%v", pass.Snapshot, err)
	}
	if pass.Snapshot.Cache.Records != 1 {
		t.Fatalf("cache stats=%+v", pass.Snapshot.Cache)
	}
	got, score, ok := cache.Lookup("E1", "go1.26/linux-arm64", []string{"main.go"})
	if !ok || got.FixHint == "" || score < .99 {
		t.Fatalf("verified fix missing: got=%+v score=%v ok=%v", got, score, ok)
	}
}

func TestRepairCycleRollbackOnRegressionAndOversizedPatch(t *testing.T) {
	cycle := NewRepairCycle(NewEscalator(), nil, nil)
	rollbacks := 0
	cycle.SetRollback(func(string) error { rollbacks++; return nil })

	res, err := cycle.Report(RepairAttempt{
		StrategyID: "A", FailureFingerprint: "E", Regression: true,
		Verification: failedReceipt(), PatchNumstat: "1\t0\ta.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rollbacks != 1 || !res.Snapshot.Recovery.Succeeded || res.Snapshot.Rollbacks != 1 {
		t.Fatalf("regression recovery=%+v rollbacks=%d", res.Snapshot.Recovery, rollbacks)
	}

	cycle.SetPatchPolicy(PatchPolicy{MaxFiles: 1, MaxChangedLines: 2, MaxSingleFileLines: 2})
	res, err = cycle.Report(RepairAttempt{
		StrategyID: "B", FailureFingerprint: "E", Verification: failedReceipt(),
		PatchNumstat: "5\t0\ta.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rollbacks != 2 || res.Snapshot.Patch.Allowed || !res.Snapshot.Recovery.Succeeded {
		t.Fatalf("oversize recovery=%+v patch=%+v rollbacks=%d", res.Snapshot.Recovery, res.Snapshot.Patch, rollbacks)
	}
}

func TestRepairCycleRollbackFailureBlocksState(t *testing.T) {
	cycle := NewRepairCycle(NewEscalator(), nil, nil)
	cycle.SetRollback(func(string) error { return errors.New("checkpoint conflict") })
	res, err := cycle.Report(RepairAttempt{
		StrategyID: "A", FailureFingerprint: "E", Regression: true,
		Verification: failedReceipt(), PatchNumstat: "1\t0\ta.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot.State != "rollback_blocked" || res.Snapshot.RollbackFailures != 1 || res.Snapshot.Recovery.Error == "" {
		t.Fatalf("snapshot=%+v", res.Snapshot)
	}
}
