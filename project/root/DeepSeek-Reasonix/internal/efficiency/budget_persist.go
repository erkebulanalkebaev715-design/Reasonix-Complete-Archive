package efficiency

import (
	"fmt"
	"math"

	"reasonix/internal/billing"
)

// BudgetPersistentState is the crash/restart-safe form of the APK budget.
// It preserves the hard KZT envelope AND already-accounted spend so restarting
// the Android/backend process cannot silently reset the user's money limit.
type BudgetPersistentState struct {
	Config        BudgetConfig `json:"config"`
	SpentKZT      float64      `json:"spentKzt"`
	ProSpentKZT   float64      `json:"proSpentKzt"`
	UnpricedCalls int          `json:"unpricedCalls"`
}

func cloneBudgetConfig(c BudgetConfig) BudgetConfig {
	out := c
	out.FXKZTPerUnit = make(map[string]float64, len(c.FXKZTPerUnit))
	for k, v := range c.FXKZTPerUnit {
		out.FXKZTPerUnit[k] = v
	}
	return out
}

func (g *Governor) PersistentState() BudgetPersistentState {
	if g == nil {
		return BudgetPersistentState{Config: DefaultBudgetConfig()}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return BudgetPersistentState{
		Config:        cloneBudgetConfig(g.cfg),
		SpentKZT:      g.spent.Float64(),
		ProSpentKZT:   g.proSpent.Float64(),
		UnpricedCalls: g.unpriced,
	}
}

func validatePersistedMoney(v float64, name string) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return fmt.Errorf("%s must be a finite non-negative value", name)
	}
	return nil
}

// RestorePersistentState restores policy and ledger without generating spend.
// Invalid/corrupt state is rejected instead of weakening a hard budget.
func (g *Governor) RestorePersistentState(st BudgetPersistentState) (BudgetSnapshot, error) {
	if g == nil {
		return BudgetSnapshot{}, fmt.Errorf("budget governor is nil")
	}
	if err := ValidateConfig(st.Config); err != nil {
		return BudgetSnapshot{}, err
	}
	if err := validatePersistedMoney(st.SpentKZT, "spentKzt"); err != nil {
		return BudgetSnapshot{}, err
	}
	if err := validatePersistedMoney(st.ProSpentKZT, "proSpentKzt"); err != nil {
		return BudgetSnapshot{}, err
	}
	if st.ProSpentKZT > st.SpentKZT+1e-9 {
		return BudgetSnapshot{}, fmt.Errorf("proSpentKzt cannot exceed spentKzt")
	}
	if st.UnpricedCalls < 0 {
		return BudgetSnapshot{}, fmt.Errorf("unpricedCalls cannot be negative")
	}

	cfg := normalizeConfig(st.Config)
	g.mu.Lock()
	g.cfg = cfg
	g.spent = billing.NewAmountFromFloat(st.SpentKZT)
	g.proSpent = billing.NewAmountFromFloat(st.ProSpentKZT)
	g.unpriced = st.UnpricedCalls
	snap := g.snapshotLocked()
	g.mu.Unlock()
	return snap, nil
}
