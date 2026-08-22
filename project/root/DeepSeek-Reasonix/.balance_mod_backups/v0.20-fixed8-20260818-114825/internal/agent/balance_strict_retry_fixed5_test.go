package agent

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func TestBalanceStrictPreCallUsesCurrentAttemptEnvelope(t *testing.T) {
	a := New(&spendingProvider{max: 0}, tool.NewRegistry(), NewSession("sys"), Options{
		Pricing:    &provider.Pricing{Input: 0.44, Output: 1.32, Currency: "USD"},
		TaskBudget: TaskBudget{Cost: 0.021673638353670432},
	}, event.Discard)
	a.SetStrictPreCallBudget(true)
	req := provider.Request{
		Messages:  []provider.Message{{Role: provider.RoleUser, Content: strings.Repeat("x", 8000)}},
		MaxTokens: 64,
	}
	if err := a.applyStrictPreCallBudget(context.Background(), &req); err != nil {
		t.Fatalf("current-attempt hard budget rejected fitting request: %v", err)
	}
	if req.MaxTokens <= 0 || req.MaxTokens > 64 {
		t.Fatalf("MaxTokens=%d, want 1..64", req.MaxTokens)
	}
}

func TestBalanceStrictReasoningFallbackSuppressed(t *testing.T) {
	a := &Agent{}
	a.SetStrictPreCallBudget(true)
	if _, ok := a.runMissingReasoningFallback(context.Background(), 1, nil, "strict", 1, nil); ok {
		t.Fatal("strict hard-budget mode must not spawn a hidden reasoning fallback provider call")
	}
}
