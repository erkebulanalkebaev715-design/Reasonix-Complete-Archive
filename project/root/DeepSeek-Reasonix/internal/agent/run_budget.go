package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// TaskBudget bounds one task on the axes its failures are reported in, and
// every axis ships off: stopping a task is the user's call. Tokens is the one
// that generalizes — a slow expensive loop accumulates them and so does a fast
// empty one, where wall clock catches only the first and money is not portable
// across models.
type TaskBudget struct {
	Cost   float64
	Wall   time.Duration
	Tokens int
}

// normalizeTaskBudget reads a negative value as unset, so a disabled axis and
// an unconfigured one behave identically.
func normalizeTaskBudget(b TaskBudget) TaskBudget {
	if b.Cost < 0 {
		b.Cost = 0
	}
	if b.Wall < 0 {
		b.Wall = 0
	}
	if b.Tokens < 0 {
		b.Tokens = 0
	}
	return b
}

// runBudget accumulates what a turn has actually spent. Rounds are a poor proxy
// for it: the same hundred of them cost minutes or hours depending on what each
// one read and how long the model thought, and the failures worth stopping are
// reported in hours and tokens, never in rounds.
type runBudget struct {
	started       time.Time
	rounds        int
	requests      int
	promptTokens  int
	outputTokens  int
	cost          float64
	pricedRounds  int
	unpricedTurns bool
	// limit is configuration, not accumulation: it survives the reset that
	// starts a new task.
	limit TaskBudget
}

// observe folds one round's provider usage into the turn's running total.
// A round whose usage never arrived still counts as a round, so the axis never
// reads cheaper than the turn actually was.
func (b *runBudget) observe(usage *provider.Usage, pricing *provider.Pricing) {
	b.rounds++
	if usage == nil {
		return
	}
	b.requests += usageRequestCount(usage)
	b.promptTokens += usage.PromptTokens
	b.outputTokens += usage.CompletionTokens
	if pricing == nil {
		b.unpricedTurns = true
		return
	}
	b.cost += pricing.Cost(usage)
	b.pricedRounds++
}

func (b *runBudget) elapsed() time.Duration {
	if b.started.IsZero() {
		return 0
	}
	return time.Since(b.started)
}

// totals is the shadow reading for one scope: counts and money, never content.
func (b *runBudget) totals() event.RunBudgetTotals {
	return event.RunBudgetTotals{
		Rounds:       b.rounds,
		Requests:     b.requests,
		PromptTokens: b.promptTokens,
		OutputTokens: b.outputTokens,
		Cost:         b.cost,
		Priced:       !b.unpricedTurns && b.pricedRounds > 0,
		ElapsedMs:    b.elapsed().Milliseconds(),
	}
}

// exceeded names the first axis the task has spent past, or "" while inside
// the budget. Cost only counts when the turn was actually priced: an unpriced
// model reads as free, and a free reading must never look like a crossing.
func (b *runBudget) exceeded(limit TaskBudget) (axis, detail string) {
	if limit.Tokens > 0 {
		if used := b.promptTokens + b.outputTokens; used >= limit.Tokens {
			return "token", fmt.Sprintf("task used %d tokens, reaching the %d budget", used, limit.Tokens)
		}
	}
	if limit.Cost > 0 && b.pricedRounds > 0 && !b.unpricedTurns && b.cost >= limit.Cost {
		return "cost", fmt.Sprintf("task spend %.4f reached the %.4f budget", b.cost, limit.Cost)
	}
	if limit.Wall > 0 {
		if elapsed := b.elapsed(); elapsed >= limit.Wall {
			return "time", fmt.Sprintf("task ran %s, past the %s budget",
				elapsed.Round(time.Second), limit.Wall)
		}
	}
	return "", ""
}

// taskBudgetLimit resolves this turn's bound: a host-injected budget wins over
// the configured one, which is how an unattended loop gets a ceiling while
// ordinary chat keeps none.
func (a *Agent) taskBudgetLimit(ctx context.Context) TaskBudget {
	if b, ok := taskBudgetFromContext(ctx); ok {
		return b
	}
	return a.task.budget.limit
}

// ResetTaskBudget starts a fresh user-approved spend slice without touching
// Delivery evidence or the persisted Goal usage totals. Callers use this only
// after a resumable explicit-budget pause, while no Agent Run is active.
func (a *Agent) ResetTaskBudget() {
	a.task.budget = runBudget{limit: a.task.budget.limit}
}

// observeRunBudget folds a round into both scopes and reports them.
func (a *Agent) observeRunBudget(state *turnRuntime, usage *provider.Usage) {
	if state == nil {
		return
	}
	state.budget.observe(usage, a.svc.pricing)
	if a.task.budget.started.IsZero() {
		a.task.budget.started = state.budget.started
	}
	a.task.budget.observe(usage, a.svc.pricing)
	currency := ""
	if a.svc.pricing != nil {
		currency = a.svc.pricing.Symbol()
	}
	event.RecordRunBudget(a.svc.sink, event.RunBudgetSample{
		Turn:     state.budget.totals(),
		Task:     a.task.budget.totals(),
		Currency: currency,
	})
}

type taskBudgetContextKey struct{}

// WithTaskBudget overrides a run's task budget for one turn. The agent serving
// an unattended loop and the one serving chat are the same instance, so the
// bound is a property of the turn, not of construction.
func WithTaskBudget(ctx context.Context, b TaskBudget) context.Context {
	return context.WithValue(ctx, taskBudgetContextKey{}, normalizeTaskBudget(b))
}

func taskBudgetFromContext(ctx context.Context) (TaskBudget, bool) {
	if ctx == nil {
		return TaskBudget{}, false
	}
	b, ok := ctx.Value(taskBudgetContextKey{}).(TaskBudget)
	return b, ok
}

// strictTaskBudgetLimit preserves the controller's global hard cost ceiling even
// when a queued/automatic turn supplies only token or wall limits. A context
// budget may tighten an axis but must never erase the configured hard bound.
func (a *Agent) strictTaskBudgetLimit(ctx context.Context) TaskBudget {
	base := normalizeTaskBudget(a.task.budget.limit)
	override, ok := taskBudgetFromContext(ctx)
	if !ok {
		return base
	}
	override = normalizeTaskBudget(override)
	tighter := func(global, local float64) float64 {
		switch {
		case global > 0 && local > 0:
			return math.Min(global, local)
		case global > 0:
			return global
		default:
			return local
		}
	}
	base.Cost = tighter(base.Cost, override.Cost)
	if base.Tokens > 0 && override.Tokens > 0 {
		base.Tokens = min(base.Tokens, override.Tokens)
	} else if base.Tokens <= 0 {
		base.Tokens = override.Tokens
	}
	if base.Wall > 0 && override.Wall > 0 {
		base.Wall = min(base.Wall, override.Wall)
	} else if base.Wall <= 0 {
		base.Wall = override.Wall
	}
	return base
}

// SetTaskCostBudget updates the configured cost ceiling for subsequent Runs.
// The host must call it only while this Agent is idle; Controller enforces that
// boundary for HTTP/APK callers. Existing task usage is preserved so lowering a
// budget never erases already-spent cost.
func (a *Agent) SetTaskCostBudget(cost float64) {
	if a == nil {
		return
	}
	if cost < 0 {
		cost = 0
	}
	a.task.budget.limit.Cost = cost
}

// SetStrictPreCallBudget toggles the host-owned pre-network budget guard. The
// controller exposes this only as an optional idle-only capability; ordinary
// Reasonix users that never enable Balance hard budgets keep historical behavior.
func (a *Agent) SetStrictPreCallBudget(enabled bool) {
	if a == nil {
		return
	}
	a.strictPreCallBudget.Store(enabled)
}

// StrictPreCallBudget reports whether provider requests are currently guarded.
func (a *Agent) StrictPreCallBudget() bool {
	return a != nil && a.strictPreCallBudget.Load()
}

// strict-one-paid-attempt-v0.20-fixed9: strict hard-budget mode disables
// hidden provider/header retries and agent stream/reasoning replays. Therefore
// this guard admits exactly the current paid attempt against the current
// remaining allowance. A later paid attempt must return through host admission
// after the ledger has observed the previous attempt.
const strictPreCallInputChargeMultiplier = 2.0

// strictPreCallCompletionReserve keeps the strict pre-call cap strictly below
// the provider ceiling: at the ceiling a guard would be indistinguishable from
// the uncapped server default and leaves no room for the completion boundary.
const strictPreCallCompletionReserve = 8 * 1024

// applyStrictPreCallBudget caps req.MaxTokens before provider network I/O. The
// prompt estimate is intentionally an upper bound based on the complete JSON
// request byte size plus framing headroom. Byte count is conservative for the
// byte-backed tokenizers used by supported providers and avoids needing another
// tokenizer/model/API call on Android.
func (a *Agent) applyStrictPreCallBudget(ctx context.Context, req *provider.Request) error {
	if a == nil || req == nil || !a.StrictPreCallBudget() {
		return nil
	}
	limit := a.strictTaskBudgetLimit(ctx)
	if limit.Cost <= 0 && limit.Tokens <= 0 {
		// Strict mode is only enabled by a host that promised a hard ceiling.
		// Zero therefore means no allowance remains, not "unconfigured".
		return fmt.Errorf("pre-call hard budget has no remaining allowance")
	}

	promptUpper := providerRequestTokenUpperBound(*req)
	cap := req.MaxTokens
	if cap <= 0 {
		// Unbounded output means the server would apply its own ceiling, so the
		// strict guard's hard cap must be the provider ceiling itself.
		cap = provider.DeepSeekMaxOutputTokens
	}
	setCap := func(v int) {
		if v <= 0 {
			return
		}
		if v < cap {
			cap = v
		}
	}

	if limit.Tokens > 0 {
		used := a.task.budget.promptTokens + a.task.budget.outputTokens
		remaining := limit.Tokens - used
		if remaining <= 0 {
			return fmt.Errorf("pre-call token budget exhausted: used %d of %d", used, limit.Tokens)
		}
		share := remaining
		if share <= promptUpper {
			return fmt.Errorf("pre-call token budget cannot safely admit request: current-attempt allowance %d <= prompt upper bound %d", share, promptUpper)
		}
		setCap(share - promptUpper)
	}

	if limit.Cost > 0 {
		pricing := a.svc.pricing
		if pricing == nil {
			return fmt.Errorf("pre-call cost budget cannot be enforced without model pricing")
		}
		remaining := limit.Cost - a.task.budget.cost
		if remaining <= 0 {
			return fmt.Errorf("pre-call cost budget exhausted: %.6f of %.6f spent", a.task.budget.cost, limit.Cost)
		}
		share := remaining
		inputRate := math.Max(pricing.Input, pricing.CacheHit)
		inputUpper := float64(promptUpper) * inputRate * strictPreCallInputChargeMultiplier / 1e6
		if inputUpper >= share {
			return fmt.Errorf("pre-call cost budget cannot safely admit prompt: estimated input %.6f >= current-attempt allowance %.6f %s", inputUpper, share, pricing.Symbol())
		}
		if pricing.Output > 0 {
			out := int(math.Floor((share - inputUpper) * 1e6 / pricing.Output))
			if out <= 0 {
				return fmt.Errorf("pre-call cost budget leaves no completion tokens")
			}
			setCap(out)
		}
	}

	// Clamp to the provider ceiling. Exactly-at-ceiling is indistinguishable
	// from the uncapped default, so reserve a completion margin below it; the
	// budget derivation governs whenever it is the tighter bound.
	if cap >= provider.DeepSeekMaxOutputTokens {
		cap = provider.DeepSeekMaxOutputTokens - strictPreCallCompletionReserve
	}

	if cap > 0 {
		req.MaxTokens = cap
	}
	return nil
}

func providerRequestTokenUpperBound(req provider.Request) int {
	clone := req
	clone.MaxTokens = 0
	b, err := json.Marshal(clone)
	if err != nil {
		// Fail closed with a deliberately large bound rather than pretending an
		// unserializable request is cheap.
		return 1 << 30
	}
	overhead := 4096 + 128*(len(req.Messages)+len(req.Tools))
	if n := len(b) + overhead; n > 0 {
		return n
	}
	return 1
}
