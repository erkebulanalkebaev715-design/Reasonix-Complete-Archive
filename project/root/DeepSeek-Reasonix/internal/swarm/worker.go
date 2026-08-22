package swarm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// workerConfig is what a worker needs to execute one real Reasonix turn.
type workerConfig struct {
	Resolver Resolver
	Registry *tool.Registry // full app registry; the worker scope is derived from it
	Profiles map[string]Profile
	Default  string
	Sink     event.Sink // swarm sink; worker agent events forward through it
}

// workerOutcome is the structured result of one worker run.
type workerOutcome struct {
	Result    *TaskResult
	Failure   *TaskFailure
	ModelRef  string
	Provider  string
	Requests  int
	CostSpent float64
	Tokens    int
	Cancelled bool
}

// workerResultCollector forwards every agent event to the swarm sink while
// deriving the structured evidence the worker reports.
type workerResultCollector struct {
	sink    event.Sink
	swarmID string
	taskID  string
	worker  string

	finalText  strings.Builder
	toolCount  int
	toolErrors int
	usage      provider.Usage
	modelRef   string
	provider   string
	reasoning  bool
}

func (c *workerResultCollector) Emit(e event.Event) {
	if c.sink != nil {
		// Forward as-is; the swarm sub-events are emitted by the orchestrator.
		c.sink.Emit(e)
	}
	switch e.Kind {
	case event.Text, event.Message:
		if e.Kind == event.Text {
			c.finalText.WriteString(e.Text)
		} else if e.Text != "" {
			c.finalText.Reset()
			c.finalText.WriteString(e.Text)
		}
	case event.ToolResult:
		c.toolCount++
		if e.Tool.Err != "" {
			c.toolErrors++
		}
	case event.Usage:
		if e.Usage != nil {
			c.usage.PromptTokens += e.Usage.PromptTokens
			c.usage.CompletionTokens += e.Usage.CompletionTokens
			c.usage.CacheHitTokens += e.Usage.CacheHitTokens
			c.usage.CacheWriteTokens += e.Usage.CacheWriteTokens
			c.usage.RequestCount += e.Usage.RequestCount
			if e.ModelRef != "" {
				c.modelRef = e.ModelRef
			}
		}
	case event.Reasoning:
		c.reasoning = true
	}
}

func (c *workerResultCollector) snapshot() *TaskResult {
	res := &TaskResult{
		Summary: strings.TrimSpace(c.finalText.String()),
		Evidence: []Evidence{
			{Kind: EvidenceProvider, Result: "pass", Ref: c.modelRef, At: time.Now()},
		},
	}
	if c.toolCount > 0 {
		res.Evidence = append(res.Evidence, Evidence{Kind: EvidenceRuntime, Result: "pass", Ref: fmt.Sprintf("tools=%d", c.toolCount), At: time.Now()})
	}
	if res.Summary != "" {
		res.Evidence = append(res.Evidence, Evidence{Kind: EvidenceReadback, Result: "pass", At: time.Now()})
	}
	if c.toolErrors > 0 {
		res.Findings = append(res.Findings, Finding{Kind: FindingError, Summary: fmt.Sprintf("%d tool call(s) failed", c.toolErrors)})
	}
	return res
}

// runWorker executes one bounded task through the real Reasonix agent turn
// primitive. The worker is provider-agnostic and offline-testable: the Resolver
// decides which provider/model runs the turn.
func runWorker(ctx context.Context, cfg workerConfig, task *Task, profile Profile) workerOutcome {
	sink := &workerResultCollector{sink: cfg.Sink, swarmID: task.ID, taskID: task.ID, worker: task.WorkerID}
	prov, pricing, modelRef, providerName, err := cfg.Resolver.Resolve(profile.ModelPreference)
	if err != nil {
		failure := &TaskFailure{Class: FailurePermanent, Message: err.Error(), At: time.Now()}
		return workerOutcome{Failure: failure, ModelRef: modelRef, Provider: providerName}
	}

	reg := scopedRegistry(cfg.Registry, profile.AllowedTools, nil)
	sess := agent.NewSession(workerSystemPrompt(profile))
	opts := agent.Options{
		MaxSteps:          profile.MaxSteps,
		Temperature:       0,
		TaskBudget:        workerTaskBudget(profile),
		Pricing:           pricing,
		ModelRef:          modelRef,
		ContextWindow:     profile.ContextWindow,
		ReadOnlyExecution: profile.ReadOnly,
	}
	a := agent.New(prov, reg, sess, opts, sink)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if profile.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, profile.Timeout)
		defer cancel()
	}

	runErr := a.Run(runCtx, task.Objective)
	outcome := workerOutcome{
		Result:    sink.snapshot(),
		ModelRef:  modelRef,
		Provider:  providerName,
		Requests:  sink.usage.RequestCount,
		CostSpent: workerCost(sink.usage, pricing),
		Tokens:    sink.usage.TotalTokens,
		Cancelled: runCtx.Err() == context.Canceled,
	}
	outcome.Result.Tests = collectTestEvidence(runErr)
	if runErr != nil {
		outcome.Failure = classifyWorkerFailure(runErr)
	} else if missingRequiredEvidence(outcome.Result, profile.RequiredEvidence) {
		outcome.Failure = &TaskFailure{Class: FailureNoProgress, Message: "required evidence not produced", At: time.Now()}
	}
	return outcome
}

func collectTestEvidence(runErr error) []TestResult {
	if runErr != nil {
		return []TestResult{{Name: "turn", Pass: false}}
	}
	return []TestResult{{Name: "turn", Pass: true}}
}

// classifyWorkerFailure maps a worker run error to a failure class. This is a
// conservative host-side classifier; the controller/agent already gates
// terminal errors, so unknown failures are treated as temporary (retryable)
// rather than permanent to avoid masking a transient provider blip.
func classifyWorkerFailure(err error) *TaskFailure {
	msg := err.Error()
	class := FailureTemporary
	retryable := true
	switch {
	case isCancellation(err):
		class, retryable = FailureCancelled, false
	case isDeadline(err):
		class, retryable = FailureTimeout, true
	case strings.Contains(strings.ToLower(msg), "budget") ||
		strings.Contains(strings.ToLower(msg), "insufficient funds") ||
		strings.Contains(strings.ToLower(msg), "no remaining allowance"):
		class, retryable = FailureBudgetStop, false
	case strings.Contains(strings.ToLower(msg), "no progress") ||
		strings.Contains(strings.ToLower(msg), "loop guard"):
		class, retryable = FailureNoProgress, true
	case strings.Contains(strings.ToLower(msg), "approval") ||
		strings.Contains(strings.ToLower(msg), "ask") ||
		strings.Contains(strings.ToLower(msg), "permission denied"):
		class, retryable = FailureApprovalWait, true
	case strings.Contains(strings.ToLower(msg), "unknown tool") ||
		strings.Contains(strings.ToLower(msg), "no such tool"):
		class, retryable = FailureToolMissing, false
	case strings.Contains(strings.ToLower(msg), "schema") ||
		strings.Contains(strings.ToLower(msg), "validation"):
		class, retryable = FailureSchemaError, false
	case strings.Contains(strings.ToLower(msg), "authentication") ||
		strings.Contains(strings.ToLower(msg), "http 40") ||
		strings.Contains(strings.ToLower(msg), "http 50"):
		class, retryable = FailureProviderError, true
	}
	return &TaskFailure{Class: class, Message: msg, Retryable: retryable, At: time.Now()}
}

func isCancellation(err error) bool {
	return err != nil && (err == context.Canceled ||
		strings.Contains(err.Error(), "context canceled"))
}

func isDeadline(err error) bool {
	return err != nil && (err == context.DeadlineExceeded ||
		strings.Contains(err.Error(), "context deadline"))
}

// workerTaskBudget derives the agent task budget from a profile. A zero worker
// cost means the native DefaultTaskBudget governs; the swarm's own total budget
// still caps the run at the orchestrator.
func workerTaskBudget(profile Profile) agent.TaskBudget {
	return agent.TaskBudget{Cost: profile.BudgetCost, Tokens: profile.BudgetTokens}
}

// workerCost charges the observed usage against the provider rate card in the
// card's currency, matching the billing package's per-1M-token convention.
func workerCost(u provider.Usage, p *provider.Pricing) float64 {
	if p == nil {
		return 0
	}
	input := float64(u.PromptTokens+u.CacheWriteTokens) * p.Input
	output := float64(u.CompletionTokens) * p.Output
	return (input + output) / 1e6
}

func workerSystemPrompt(profile Profile) string {
	base := "You are a Reasonix worker agent.\n" + profile.Instructions
	if len(profile.AllowedTools) > 0 {
		base += "\nAllowed tools: " + strings.Join(profile.AllowedTools, ", ")
	}
	return base
}

// scopedRegistry clones the subset of the full registry the profile allows.
// Cloning keeps each worker's provider schema bounded and prevents a worker
// from reaching host-level tools (task/fleet/use_capability) that belong to
// the parent session. nil visible set keeps all registered tools executable;
// the provider schema is restricted to the cloned set.
func scopedRegistry(full *tool.Registry, allowed []string, _ []string) *tool.Registry {
	reg := tool.NewRegistry()
	if full == nil {
		return reg
	}
	if len(allowed) == 0 {
		// No explicit restriction: clone the read-only surface only so a worker
		// cannot mutate the host, and leave host-delegation tools out.
		allowed = readOnlyTools
	}
	for _, name := range allowed {
		t, ok := full.Get(name)
		if !ok {
			continue
		}
		reg.Add(t)
	}
	return reg
}
