package efficiency

import (
	"context"
	"testing"

	"reasonix/internal/provider"
	mockprovider "reasonix/internal/provider/mock"
)

func mockRepairMarker(t *testing.T, p provider.Provider) string {
	t.Helper()
	ch, err := p.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatal(err)
	}
	for c := range ch {
		if c.Type == provider.ChunkText {
			return c.Text
		}
	}
	t.Fatal("mock repair scenario emitted no marker")
	return ""
}

func TestRepairCycleDrivenByOfflineMockProvider(t *testing.T) {
	p, err := mockprovider.New(provider.Config{Name: "balance-mock-repair", Extra: map[string]any{"scenario": "repair-cycle"}})
	if err != nil {
		t.Fatal(err)
	}
	cycle := NewRepairCycle(NewEscalator(), nil, nil)
	failed := VerificationReceipt{RequiredChecks: 2, ChecksPassed: 1, ChecksFailed: 1, BuildObserved: true, BuildExitCode: 1}
	passed := VerificationReceipt{RequiredChecks: 2, ChecksPassed: 2, BuildObserved: true, BuildExitCode: 0}

	marker := mockRepairMarker(t, p)
	if marker != "MOCK_REPAIR_FAIL_A" {
		t.Fatal(marker)
	}
	r, err := cycle.Report(RepairAttempt{StrategyID: "A", FailureFingerprint: "E1", PatchNumstat: "1\t0\ta.go", Verification: failed})
	if err != nil || r.Snapshot.LastRoute.Action != RouteRetryFlash {
		t.Fatalf("step A=%+v err=%v", r.Snapshot, err)
	}

	marker = mockRepairMarker(t, p)
	if marker != "MOCK_REPAIR_FAIL_B" {
		t.Fatal(marker)
	}
	r, err = cycle.Report(RepairAttempt{StrategyID: "B", FailureFingerprint: "E1", PatchNumstat: "1\t0\ta.go", Verification: failed})
	if err != nil || r.Snapshot.LastRoute.Action != RouteDiagnosePro {
		t.Fatalf("step B=%+v err=%v", r.Snapshot, err)
	}

	marker = mockRepairMarker(t, p)
	if marker != "MOCK_PRO_DIAGNOSIS" {
		t.Fatal(marker)
	}
	r, err = cycle.Report(RepairAttempt{StrategyID: "pro-diagnosis", FailureFingerprint: "E1", Verification: failed})
	if err != nil || r.Snapshot.LastRoute.Action != RouteExecuteFlash {
		t.Fatalf("step Pro=%+v err=%v", r.Snapshot, err)
	}

	marker = mockRepairMarker(t, p)
	if marker != "MOCK_REPAIR_PASS" {
		t.Fatal(marker)
	}
	r, err = cycle.Report(RepairAttempt{StrategyID: "C", ResolvedFingerprint: "E1", PatchNumstat: "1\t0\ta.go", Verification: passed, Finalization: true})
	if err != nil || r.Snapshot.LastRoute.Action != RouteFinalize || r.Snapshot.State != "done" {
		t.Fatalf("step pass=%+v err=%v", r.Snapshot, err)
	}
}
