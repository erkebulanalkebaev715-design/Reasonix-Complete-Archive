package efficiency

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"reasonix/internal/billing"
)

// BudgetConfig is the APK-facing budget policy. BudgetKZT is the user's hard
// task/session envelope in tenge. FXKZTPerUnit maps a provider billing currency
// (USD, CNY, ...) to KZT per one unit of that currency.
type BudgetConfig struct {
	BudgetKZT      float64            `json:"budgetKzt"`
	ReservePercent int                `json:"reservePercent"`
	ProMaxPercent  int                `json:"proMaxPercent"`
	HardStop       bool               `json:"hardStop"`
	FXKZTPerUnit   map[string]float64 `json:"fxKztPerUnit,omitempty"`
}

// BudgetSnapshot is content-free runtime telemetry suitable for the APK.
type BudgetSnapshot struct {
	Enabled         bool               `json:"enabled"`
	BudgetKZT       float64            `json:"budgetKzt"`
	SpentKZT        float64            `json:"spentKzt"`
	RemainingKZT    float64            `json:"remainingKzt"`
	ReserveKZT      float64            `json:"reserveKzt"`
	RegularLimitKZT float64            `json:"regularLimitKzt"`
	ProSpentKZT     float64            `json:"proSpentKzt"`
	ProLimitKZT     float64            `json:"proLimitKzt"`
	ReservePercent  int                `json:"reservePercent"`
	ProMaxPercent   int                `json:"proMaxPercent"`
	HardStop        bool               `json:"hardStop"`
	Exhausted       bool               `json:"exhausted"`
	ProLimitReached bool               `json:"proLimitReached"`
	UnpricedCalls   int                `json:"unpricedCalls"`
	FXKZTPerUnit    map[string]float64 `json:"fxKztPerUnit,omitempty"`
	Enforcement     string             `json:"enforcement"`
}

// Governor owns the user-visible KZT budget. It intentionally does not own
// model routing: routing consults Snapshot/CanSpend so the budget remains the
// single source of truth while the router stays replaceable.
type Governor struct {
	mu sync.Mutex

	cfg BudgetConfig

	spent    billing.Amount // KZT
	proSpent billing.Amount // KZT
	unpriced int
}

// DefaultBudgetConfig returns the APK policy defaults. Decode JSON on top of
// this value when omitted fields should keep the defaults.
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{ReservePercent: 15, ProMaxPercent: 25, HardStop: true}
}

func NewGovernor() *Governor {
	g := &Governor{}
	g.cfg = normalizeConfig(DefaultBudgetConfig())
	return g
}

func normalizeConfig(c BudgetConfig) BudgetConfig {
	if math.IsNaN(c.BudgetKZT) || math.IsInf(c.BudgetKZT, 0) || c.BudgetKZT < 0 {
		c.BudgetKZT = 0
	}
	if c.ReservePercent < 0 {
		c.ReservePercent = 0
	}
	if c.ReservePercent > 80 {
		c.ReservePercent = 80
	}
	if c.ProMaxPercent < 0 {
		c.ProMaxPercent = 0
	}
	if c.ProMaxPercent > 100 {
		c.ProMaxPercent = 100
	}
	clean := make(map[string]float64, len(c.FXKZTPerUnit))
	for cur, rate := range c.FXKZTPerUnit {
		cur = billing.NormalizeCurrency(cur)
		if cur == "KZT" {
			clean[cur] = 1
			continue
		}
		if cur == "" || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			continue
		}
		clean[cur] = rate
	}
	if c.BudgetKZT > 0 && clean["KZT"] == 0 {
		clean["KZT"] = 1
	}
	c.FXKZTPerUnit = clean
	return c
}

// Configure replaces policy and starts a fresh budget ledger. The HTTP layer
// only calls this while the controller is idle, so a budget cannot move under
// an in-flight model request.
func (g *Governor) Configure(c BudgetConfig) BudgetSnapshot {
	if g == nil {
		return BudgetSnapshot{}
	}
	c = normalizeConfig(c)
	g.mu.Lock()
	g.cfg = c
	g.spent = billing.Zero
	g.proSpent = billing.Zero
	g.unpriced = 0
	snap := g.snapshotLocked()
	g.mu.Unlock()
	return snap
}

// ResetSpend preserves policy and clears only runtime usage.
func (g *Governor) ResetSpend() BudgetSnapshot {
	if g == nil {
		return BudgetSnapshot{}
	}
	g.mu.Lock()
	g.spent = billing.Zero
	g.proSpent = billing.Zero
	g.unpriced = 0
	snap := g.snapshotLocked()
	g.mu.Unlock()
	return snap
}

// ObserveQuote accounts one host-generated usage quote. Individual Usage
// events are consumed exactly once by the serve broadcaster, so this is an
// additive ledger rather than a sample/max accumulator.
func (g *Governor) ObserveQuote(q *billing.CostQuote, modelRef string) BudgetSnapshot {
	if g == nil {
		return BudgetSnapshot{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if q == nil || !q.CostComplete {
		g.unpriced++
		return g.snapshotLocked()
	}
	money := q.Original
	cur := billing.NormalizeCurrency(money.Currency)
	rate := g.cfg.FXKZTPerUnit[cur]
	if cur == "KZT" && rate == 0 {
		rate = 1
	}
	if rate <= 0 {
		g.unpriced++
		return g.snapshotLocked()
	}
	amount := money.AmountValue()
	kzt := amount.MulRate(rate)
	if kzt < 0 {
		kzt = 0
	}
	g.spent = g.spent.Add(kzt)
	if isProModel(modelRef, q.ModelRef) {
		g.proSpent = g.proSpent.Add(kzt)
	}
	return g.snapshotLocked()
}

func isProModel(refs ...string) bool {
	for _, ref := range refs {
		ref = strings.ToLower(strings.TrimSpace(ref))
		if strings.Contains(ref, "deepseek-pro") || strings.Contains(ref, "v4-pro") || strings.HasSuffix(ref, "/pro") {
			return true
		}
	}
	return false
}

// CanSpend is the future pre-call gate used by the Flash/Pro router. estimatedKZT
// is a conservative upper estimate for the next provider call. finalization may
// spend the reserved tail; ordinary work may not.
func (g *Governor) CanSpend(modelClass string, estimatedKZT float64, finalization bool) (bool, string) {
	if g == nil {
		return true, ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cfg.BudgetKZT <= 0 || !g.cfg.HardStop {
		return true, ""
	}
	if estimatedKZT < 0 || math.IsNaN(estimatedKZT) || math.IsInf(estimatedKZT, 0) {
		estimatedKZT = 0
	}
	total := billing.NewAmountFromFloat(g.cfg.BudgetKZT)
	reserve := total.MulRate(float64(g.cfg.ReservePercent) / 100)
	ceiling := total
	if !finalization {
		ceiling = total.Add(-reserve)
	}
	next := g.spent.Add(billing.NewAmountFromFloat(estimatedKZT))
	if next > ceiling {
		if finalization {
			return false, "task budget would be exceeded"
		}
		return false, "regular budget is exhausted; reserved budget is kept for final verification/fix"
	}
	if strings.EqualFold(strings.TrimSpace(modelClass), "pro") && g.cfg.ProMaxPercent > 0 {
		proLimit := total.MulRate(float64(g.cfg.ProMaxPercent) / 100)
		if g.proSpent.Add(billing.NewAmountFromFloat(estimatedKZT)) > proLimit {
			return false, "Pro budget cap would be exceeded"
		}
	}
	return true, ""
}

func (g *Governor) Snapshot() BudgetSnapshot {
	if g == nil {
		return BudgetSnapshot{}
	}
	g.mu.Lock()
	snap := g.snapshotLocked()
	g.mu.Unlock()
	return snap
}

func (g *Governor) snapshotLocked() BudgetSnapshot {
	total := billing.NewAmountFromFloat(g.cfg.BudgetKZT)
	reserve := total.MulRate(float64(g.cfg.ReservePercent) / 100)
	regular := total.Add(-reserve)
	proLimit := total.MulRate(float64(g.cfg.ProMaxPercent) / 100)
	remaining := total.Add(-g.spent)
	if remaining < 0 {
		remaining = 0
	}
	enabled := g.cfg.BudgetKZT > 0
	fx := make(map[string]float64, len(g.cfg.FXKZTPerUnit))
	for k, v := range g.cfg.FXKZTPerUnit {
		fx[k] = v
	}
	return BudgetSnapshot{
		Enabled:         enabled,
		BudgetKZT:       total.Float64(),
		SpentKZT:        g.spent.Float64(),
		RemainingKZT:    remaining.Float64(),
		ReserveKZT:      reserve.Float64(),
		RegularLimitKZT: regular.Float64(),
		ProSpentKZT:     g.proSpent.Float64(),
		ProLimitKZT:     proLimit.Float64(),
		ReservePercent:  g.cfg.ReservePercent,
		ProMaxPercent:   g.cfg.ProMaxPercent,
		HardStop:        g.cfg.HardStop,
		Exhausted:       enabled && g.spent >= total,
		ProLimitReached: enabled && g.cfg.ProMaxPercent > 0 && g.proSpent >= proLimit,
		UnpricedCalls:   g.unpriced,
		FXKZTPerUnit:    fx,
		Enforcement:     "post-round-ledger+conditional-precall-v0.16",
	}
}

// ProviderBudget converts a KZT hard ceiling to the active provider price-book
// currency so Reasonix's existing TaskCostBudget can act as a second stop layer.
// It returns ok=false when the conversion is unknown.
func (g *Governor) ProviderBudget(currency string) (value float64, ok bool) {
	if g == nil {
		return 0, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cfg.BudgetKZT <= 0 {
		return 0, true
	}
	cur := billing.NormalizeCurrency(currency)
	rate := g.cfg.FXKZTPerUnit[cur]
	if cur == "KZT" && rate == 0 {
		rate = 1
	}
	if rate <= 0 {
		return 0, false
	}
	return g.cfg.BudgetKZT / rate, true
}

// RemainingProviderBudget converts the unspent global KZT envelope into the
// active provider currency. A rebuilt controller starts with a fresh local
// task accumulator, so feeding it the original total would silently re-grant
// money already consumed by the previous model instance.
func (g *Governor) RemainingProviderBudget(currency string) (value float64, ok bool) {
	if g == nil {
		return 0, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cfg.BudgetKZT <= 0 {
		return 0, true
	}
	cur := billing.NormalizeCurrency(currency)
	rate := g.cfg.FXKZTPerUnit[cur]
	if cur == "KZT" && rate == 0 {
		rate = 1
	}
	if rate <= 0 {
		return 0, false
	}
	remaining := g.cfg.BudgetKZT - g.spent.Float64()
	if remaining < 0 {
		remaining = 0
	}
	return remaining / rate, true
}

func ValidateConfig(c BudgetConfig) error {
	if c.BudgetKZT < 0 || math.IsNaN(c.BudgetKZT) || math.IsInf(c.BudgetKZT, 0) {
		return fmt.Errorf("budgetKzt must be a finite non-negative number")
	}
	if c.ReservePercent < 0 || c.ReservePercent > 80 {
		return fmt.Errorf("reservePercent must be between 0 and 80")
	}
	if c.ProMaxPercent < 0 || c.ProMaxPercent > 100 {
		return fmt.Errorf("proMaxPercent must be between 0 and 100")
	}
	for cur, rate := range c.FXKZTPerUnit {
		if strings.TrimSpace(cur) == "" || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return fmt.Errorf("fxKztPerUnit contains an invalid rate for %q", cur)
		}
	}
	return nil
}
