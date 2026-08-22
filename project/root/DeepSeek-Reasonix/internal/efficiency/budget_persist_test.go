package efficiency

import "testing"

func TestBudgetPersistentStateRestoresSpendAndHardLimit(t *testing.T) {
	g := NewGovernor()
	g.Configure(BudgetConfig{BudgetKZT: 100, ReservePercent: 10, ProMaxPercent: 25, HardStop: true, FXKZTPerUnit: map[string]float64{"KZT": 1}})
	_, err := g.RestorePersistentState(BudgetPersistentState{
		Config:   BudgetConfig{BudgetKZT: 100, ReservePercent: 10, ProMaxPercent: 25, HardStop: true, FXKZTPerUnit: map[string]float64{"KZT": 1}},
		SpentKZT: 80, ProSpentKZT: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	st := g.PersistentState()
	if st.SpentKZT != 80 || st.ProSpentKZT != 20 {
		t.Fatalf("restored state = %+v", st)
	}
	if ok, _ := g.CanSpend("flash", 11, false); ok {
		t.Fatal("regular budget reserve must survive restart")
	}
	if ok, _ := g.CanSpend("pro", 6, true); ok {
		t.Fatal("Pro cap must survive restart")
	}
}

func TestBudgetPersistentStateRejectsImpossibleLedger(t *testing.T) {
	g := NewGovernor()
	_, err := g.RestorePersistentState(BudgetPersistentState{Config: DefaultBudgetConfig(), SpentKZT: 1, ProSpentKZT: 2})
	if err == nil {
		t.Fatal("expected impossible ledger rejection")
	}
}
