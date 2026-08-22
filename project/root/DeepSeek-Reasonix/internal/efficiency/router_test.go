package efficiency

import "testing"

func TestEscalatorFlashFlashProFlash(t *testing.T) {
	r := NewEscalator()
	g := NewGovernor()
	g.Configure(BudgetConfig{BudgetKZT: 1000, ReservePercent: 15, ProMaxPercent: 25, HardStop: true, FXKZTPerUnit: map[string]float64{"KZT": 1}})

	d := r.Decide(RouteInput{FailureFingerprint: "E1", StrategyID: "A", EstimatedFlashKZT: 1}, g)
	if d.Action != RouteRetryFlash {
		t.Fatalf("first = %s", d.Action)
	}
	d = r.Decide(RouteInput{FailureFingerprint: "E1", StrategyID: "B", EstimatedFlashKZT: 1, EstimatedProKZT: 5}, g)
	if d.Action != RouteDiagnosePro {
		t.Fatalf("second = %s (%s)", d.Action, d.Reason)
	}
	d = r.Decide(RouteInput{FailureFingerprint: "E1", StrategyID: "B", EstimatedFlashKZT: 1}, g)
	if d.Action != RouteExecuteFlash {
		t.Fatalf("after pro = %s", d.Action)
	}
	d = r.Decide(RouteInput{Passed: true}, g)
	if d.Action != RouteFinalize {
		t.Fatalf("pass = %s", d.Action)
	}
}

func TestEscalatorDoesNotEscalateSameStrategy(t *testing.T) {
	r := NewEscalator()
	d := r.Decide(RouteInput{FailureFingerprint: "E1", StrategyID: "A"}, nil)
	if d.Action != RouteRetryFlash {
		t.Fatal(d.Action)
	}
	d = r.Decide(RouteInput{FailureFingerprint: "E1", StrategyID: "A"}, nil)
	if d.Action == RouteDiagnosePro {
		t.Fatal("same strategy must not justify Pro")
	}
}

func TestEscalatorBudgetBlocksPro(t *testing.T) {
	r := NewEscalator()
	g := NewGovernor()
	g.Configure(BudgetConfig{BudgetKZT: 10, ReservePercent: 0, ProMaxPercent: 10, HardStop: true, FXKZTPerUnit: map[string]float64{"KZT": 1}})
	_ = r.Decide(RouteInput{FailureFingerprint: "E", StrategyID: "A", EstimatedFlashKZT: .1}, g)
	d := r.Decide(RouteInput{FailureFingerprint: "E", StrategyID: "B", EstimatedFlashKZT: .1, EstimatedProKZT: 2}, g)
	if d.Action == RouteDiagnosePro {
		t.Fatal("Pro should be denied by cap")
	}
}
