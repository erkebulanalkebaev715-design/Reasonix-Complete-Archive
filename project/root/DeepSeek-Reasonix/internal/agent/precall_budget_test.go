package agent

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func newPreCallBudgetAgent(cost float64) *Agent {
	return New(&spendingProvider{max: 0}, tool.NewRegistry(), NewSession("sys"), Options{
		Pricing:    &provider.Pricing{Input: 1, Output: 2, Currency: "CNY"},
		TaskBudget: TaskBudget{Cost: cost},
	}, event.Discard)
}

func TestStrictPreCallBudgetCapsUnboundedOutputBeforeProviderIO(t *testing.T) {
	a := newPreCallBudgetAgent(1)
	a.SetStrictPreCallBudget(true)
	req := provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "small request"}}}
	if err := a.applyStrictPreCallBudget(context.Background(), &req); err != nil {
		t.Fatalf("pre-call guard: %v", err)
	}
	if req.MaxTokens <= 0 {
		t.Fatalf("MaxTokens = %d, want a positive hard cap", req.MaxTokens)
	}
	if req.MaxTokens >= provider.DeepSeekMaxOutputTokens {
		t.Fatalf("MaxTokens = %d, want budget-derived cap below DeepSeek ceiling", req.MaxTokens)
	}
}

func TestStrictPreCallBudgetFailsClosedWhenPromptCannotFitRetryReserve(t *testing.T) {
	a := newPreCallBudgetAgent(0.000001)
	a.SetStrictPreCallBudget(true)
	req := provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: strings.Repeat("x", 4096)}}}
	if err := a.applyStrictPreCallBudget(context.Background(), &req); err == nil {
		t.Fatal("expected hard pre-call rejection")
	}
}

func TestStrictPreCallBudgetDisabledPreservesRequest(t *testing.T) {
	a := newPreCallBudgetAgent(0.000001)
	req := provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}, MaxTokens: 777}
	if err := a.applyStrictPreCallBudget(context.Background(), &req); err != nil {
		t.Fatalf("disabled guard: %v", err)
	}
	if req.MaxTokens != 777 {
		t.Fatalf("MaxTokens = %d, want unchanged 777", req.MaxTokens)
	}
}

func TestProviderRequestTokenUpperBoundIncludesToolAndMessageBytes(t *testing.T) {
	small := providerRequestTokenUpperBound(provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}})
	big := providerRequestTokenUpperBound(provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: strings.Repeat("x", 8192)}}})
	if big <= small {
		t.Fatalf("upper bound did not grow: small=%d big=%d", small, big)
	}
}

func TestStrictPreCallBudgetZeroRemainingFailsClosed(t *testing.T) {
	a := newPreCallBudgetAgent(0)
	a.SetStrictPreCallBudget(true)
	req := provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}, MaxTokens: 1}
	if err := a.applyStrictPreCallBudget(context.Background(), &req); err == nil {
		t.Fatal("strict zero-remaining budget must fail closed")
	}
}

func TestStrictBudgetBlocksProviderSpawningDelegations(t *testing.T) {
	for _, name := range []string{"task", "read_only_task", "parallel_tasks", "fleet", "run_skill", "explore", "research", "review", "security_review"} {
		if !strictBudgetDelegationName(name) {
			t.Fatalf("%s must be blocked in hard pre-call budget mode", name)
		}
	}
	for _, name := range []string{"read_file", "grep", "read_skill", "bash"} {
		if strictBudgetDelegationName(name) {
			t.Fatalf("%s should remain available to the current agent", name)
		}
	}
}

func TestStrictPreCallBudgetForcesCoordinatorExecutorOnly(t *testing.T) {
	planner := &spendingProvider{max: 0}
	execProvider := &spendingProvider{max: 0}
	execAgent := New(execProvider, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
	coord := NewCoordinator(planner, NewSession("planner"), nil, nil, Options{}, execAgent, 0, event.Discard, nil)
	coord.SetStrictPreCallBudget(true)
	if err := coord.Run(context.Background(), "do the task"); err != nil {
		t.Fatalf("coordinator run: %v", err)
	}
	if got := planner.rounds.Load(); got != 0 {
		t.Fatalf("planner provider calls = %d, want 0 under strict hard budget", got)
	}
	if got := execProvider.rounds.Load(); got != 1 {
		t.Fatalf("executor provider calls = %d, want 1", got)
	}
}

func TestStrictPreCallBudgetContextCannotEraseGlobalCostLimit(t *testing.T) {
	a := newPreCallBudgetAgent(1)
	a.SetStrictPreCallBudget(true)
	ctx := WithTaskBudget(context.Background(), TaskBudget{Tokens: 1_000_000})
	limit := a.strictTaskBudgetLimit(ctx)
	if limit.Cost != 1 || limit.Tokens != 1_000_000 {
		t.Fatalf("merged strict limit = %+v", limit)
	}
}
