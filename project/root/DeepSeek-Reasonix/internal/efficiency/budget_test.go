package efficiency

import (
	"testing"

	"reasonix/internal/billing"
)

func quote(amount, currency, model string) *billing.CostQuote {
	return &billing.CostQuote{
		Original:     billing.Money{Amount: amount, Currency: currency},
		CostComplete: true,
		ModelRef:     model,
	}
}

func TestBudgetGovernorAccountsKZTAndPro(t *testing.T) {
	g := NewGovernor()
	g.Configure(BudgetConfig{
		BudgetKZT: 1000, ReservePercent: 15, ProMaxPercent: 25, HardStop: true,
		FXKZTPerUnit: map[string]float64{"USD": 500},
	})
	g.ObserveQuote(quote("0.10", "USD", "deepseek-flash/deepseek-v4-flash"), "")
	g.ObserveQuote(quote("0.20", "USD", "deepseek-pro/deepseek-v4-pro"), "")
	got := g.Snapshot()
	if got.SpentKZT != 150 || got.ProSpentKZT != 100 || got.RemainingKZT != 850 {
		t.Fatalf("snapshot = %+v", got)
	}
	if got.ReserveKZT != 150 || got.RegularLimitKZT != 850 || got.ProLimitKZT != 250 {
		t.Fatalf("limits = %+v", got)
	}
}

func TestBudgetGovernorReserveAndProGate(t *testing.T) {
	g := NewGovernor()
	g.Configure(BudgetConfig{
		BudgetKZT: 1000, ReservePercent: 15, ProMaxPercent: 25, HardStop: true,
		FXKZTPerUnit: map[string]float64{"USD": 500},
	})
	g.ObserveQuote(quote("1.60", "USD", "deepseek-flash/deepseek-v4-flash"), "") // 800 KZT
	if ok, _ := g.CanSpend("flash", 60, false); ok {
		t.Fatal("ordinary work crossed the 850 KZT regular ceiling")
	}
	if ok, _ := g.CanSpend("flash", 60, true); !ok {
		t.Fatal("finalization should be allowed to use reserve")
	}
	g.ResetSpend()
	g.ObserveQuote(quote("0.48", "USD", "deepseek-pro/deepseek-v4-pro"), "") // 240 KZT
	if ok, _ := g.CanSpend("pro", 20, false); ok {
		t.Fatal("Pro cap should reject 240+20 > 250 KZT")
	}
}

func TestProviderBudgetConversion(t *testing.T) {
	g := NewGovernor()
	g.Configure(BudgetConfig{BudgetKZT: 1000, FXKZTPerUnit: map[string]float64{"USD": 500}})
	got, ok := g.ProviderBudget("USD")
	if !ok || got != 2 {
		t.Fatalf("provider budget = %v ok=%v, want 2 USD", got, ok)
	}
}

func TestRemainingProviderBudgetDoesNotRegrantSpentMoney(t *testing.T) {
	g := NewGovernor()
	g.Configure(BudgetConfig{BudgetKZT: 1000, HardStop: true, FXKZTPerUnit: map[string]float64{"USD": 500}})
	g.ObserveQuote(quote("0.50", "USD", "mock/flash"), "") // 250 KZT spent.
	got, ok := g.RemainingProviderBudget("USD")
	if !ok || got != 1.5 {
		t.Fatalf("remaining provider budget = %v ok=%v, want 1.5 USD", got, ok)
	}
	if snap := g.Snapshot(); snap.Enforcement != "post-round-ledger+conditional-precall-v0.16" {
		t.Fatalf("enforcement = %q", snap.Enforcement)
	}
}
