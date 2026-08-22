package control

import (
	"context"
	"testing"
	"time"
)

func TestParseInboxTaskBudgetIsStrictAndContextScoped(t *testing.T) {
	spec, err := parseInboxTaskBudget(map[string]string{
		inboxTaskBudgetCostKey:   "2.5",
		inboxTaskBudgetTokensKey: "1200",
		inboxTaskBudgetWallMSKey: "30000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.configured || spec.budget.Cost != 2.5 || spec.budget.Tokens != 1200 || spec.budget.Wall != 30*time.Second {
		t.Fatalf("spec=%+v", spec)
	}
	if got := spec.withContext(context.Background()); got == nil {
		t.Fatal("budget context is nil")
	}
	for _, extra := range []map[string]string{
		{inboxTaskBudgetCostKey: "NaN"},
		{inboxTaskBudgetCostKey: "-1"},
		{inboxTaskBudgetTokensKey: "-1"},
		{inboxTaskBudgetWallMSKey: "999999999999999"},
	} {
		if _, err := parseInboxTaskBudget(extra); err == nil {
			t.Fatalf("invalid budget accepted: %v", extra)
		}
	}
}
