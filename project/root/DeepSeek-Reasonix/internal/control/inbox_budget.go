package control

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"reasonix/internal/agent"
)

const (
	inboxTaskBudgetCostKey   = "reasonix.task_budget.cost"
	inboxTaskBudgetTokensKey = "reasonix.task_budget.tokens"
	inboxTaskBudgetWallMSKey = "reasonix.task_budget.wall_ms"
)

type inboxTaskBudgetSpec struct {
	budget     agent.TaskBudget
	configured bool
}

func parseInboxTaskBudget(extra map[string]string) (inboxTaskBudgetSpec, error) {
	var out inboxTaskBudgetSpec
	if len(extra) == 0 {
		return out, nil
	}
	if raw := strings.TrimSpace(extra[inboxTaskBudgetCostKey]); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return out, fmt.Errorf("invalid task cost budget")
		}
		out.budget.Cost = v
		out.configured = out.configured || v > 0
	}
	if raw := strings.TrimSpace(extra[inboxTaskBudgetTokensKey]); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 || v > 1_000_000_000 {
			return out, fmt.Errorf("invalid task token budget")
		}
		out.budget.Tokens = int(v)
		out.configured = out.configured || v > 0
	}
	if raw := strings.TrimSpace(extra[inboxTaskBudgetWallMSKey]); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 || v > int64((7*24*time.Hour)/time.Millisecond) {
			return out, fmt.Errorf("invalid task wall-time budget")
		}
		out.budget.Wall = time.Duration(v) * time.Millisecond
		out.configured = out.configured || v > 0
	}
	return out, nil
}

func (s inboxTaskBudgetSpec) withContext(ctx context.Context) context.Context {
	if !s.configured {
		return ctx
	}
	return agent.WithTaskBudget(ctx, s.budget)
}
