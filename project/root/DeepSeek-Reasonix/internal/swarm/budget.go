package swarm

import (
	"fmt"
	"time"
)

// BudgetLimits is the first-class swarm budget policy. Cost values are in the
// resolved provider's billing currency; zero disables that axis.
type BudgetLimits struct {
	TotalCost           float64        `json:"totalCost,omitempty"`
	TotalTokens         int            `json:"totalTokens,omitempty"`
	WorkerCost          float64        `json:"workerCost,omitempty"`
	WorkerTokens        int            `json:"workerTokens,omitempty"`
	MaxWorkers          int            `json:"maxWorkers,omitempty"`
	ProviderConcurrency map[string]int `json:"providerConcurrency,omitempty"`
	Timeout             time.Duration  `json:"timeout,omitempty"`
}

// BudgetState is the cumulative spend ledger for one swarm run.
type BudgetState struct {
	CostSpent   float64            `json:"costSpent,omitempty"`
	TokensSpent int                `json:"tokensSpent,omitempty"`
	Requests    int                `json:"requests,omitempty"`
	Providers   map[string]float64 `json:"providers,omitempty"`
}

func (b *BudgetState) addCost(cost float64) {
	b.CostSpent += cost
}

func (b *BudgetState) addTokens(n int) {
	b.TokensSpent += n
}

func (b *BudgetState) addRequest(provider string, cost float64) {
	b.Requests++
	if b.Providers == nil {
		b.Providers = map[string]float64{}
	}
	b.Providers[provider] += cost
}

// validate enforces the minimum invariants of a configured budget.
func (l BudgetLimits) validate() error {
	if l.MaxWorkers <= 0 {
		return fmt.Errorf("swarm: MaxWorkers must be >= 1")
	}
	if l.TotalCost < 0 || l.TotalTokens < 0 || l.WorkerCost < 0 || l.WorkerTokens < 0 {
		return fmt.Errorf("swarm: budget axes must be non-negative")
	}
	if l.Timeout < 0 {
		return fmt.Errorf("swarm: budget Timeout must be non-negative")
	}
	return nil
}

// DefaultLimits is a conservative offline default: one worker at a time and a
// bounded wall clock. Real runs override these explicitly.
func DefaultLimits() BudgetLimits {
	return BudgetLimits{MaxWorkers: 1, Timeout: 30 * time.Minute}
}
