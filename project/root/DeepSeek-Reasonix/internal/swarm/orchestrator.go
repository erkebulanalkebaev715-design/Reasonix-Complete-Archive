package swarm

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/tool"
)

// Orchestrator owns the global objective and drives the whole swarm run: it
// decides usefulness and decomposition, schedules workers with bounded
// parallelism, integrates structured results, and verifies the integrated
// output. It is the single owner of the swarm's lifecycle.
type Orchestrator struct {
	Resolver    Resolver
	Registry    *tool.Registry
	Profiles    map[string]Profile
	Default     string
	Limits      BudgetLimits
	Now         func() time.Time
	MaxAttempts int

	sink      event.Sink
	state     *SwarmState
	mu        sync.Mutex
	cancel    context.CancelFunc
	planner   Planner
	persister func(*SwarmState) error
}

// NewOrchestrator builds a swarm orchestrator. Defaults fill any zero fields so
// ordinary callers get a conservative, offline-safe runtime.
func NewOrchestrator(resolver Resolver, reg *tool.Registry, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		Resolver:    resolver,
		Registry:    reg,
		Profiles:    DefaultProfiles(),
		Default:     "researcher",
		Limits:      DefaultLimits(),
		Now:         time.Now,
		MaxAttempts: 2,
		sink:        event.Discard,
	}
	for _, apply := range opts {
		apply(o)
	}
	if o.Resolver == nil {
		o.Resolver = DefaultConfigResolver()
	}
	return o
}

// Option configures an Orchestrator.
type Option func(*Orchestrator)

// WithSink sets the event sink the swarm emits its typed events through.
func WithSink(s event.Sink) Option { return func(o *Orchestrator) { o.sink = s } }

// WithProfiles overrides the worker profiles.
func WithProfiles(p map[string]Profile) Option { return func(o *Orchestrator) { o.Profiles = p } }

// WithDefaultProfile sets the fallback profile name.
func WithDefaultProfile(name string) Option { return func(o *Orchestrator) { o.Default = name } }

// WithLimits sets the swarm budget/concurrency policy.
func WithLimits(l BudgetLimits) Option { return func(o *Orchestrator) { o.Limits = l } }

// WithNow overrides the clock for deterministic tests.
func WithNow(f func() time.Time) Option { return func(o *Orchestrator) { o.Now = f } }

// WithMaxAttempts bounds retries of temporary worker failures.
func WithMaxAttempts(n int) Option {
	return func(o *Orchestrator) {
		if n > 0 {
			o.MaxAttempts = n
		}
	}
}

// WithPlanner overrides the deterministic planner with a custom one.
func WithPlanner(p Planner) Option { return func(o *Orchestrator) { o.planner = p } }

// WithPersister installs the durable swarm-state store (used by the serve
// layer). The swarm core treats persistence as best-effort.
func WithPersister(f func(*SwarmState) error) Option {
	return func(o *Orchestrator) { o.persister = f }
}

// Run executes the global objective and returns the authoritative swarm state.
func (o *Orchestrator) Run(ctx context.Context, objective string) (*SwarmState, error) {
	if o == nil {
		return nil, fmt.Errorf("swarm: nil orchestrator")
	}
	if err := o.Limits.validate(); err != nil {
		return nil, err
	}
	now := o.Now()
	o.state = &SwarmState{
		ID:        newSwarmID(now),
		Objective: objective,
		Status:    StatusPlanning,
		Tasks:     map[string]*Task{},
		CreatedAt: now,
		UpdatedAt: now,
		Budget:    BudgetState{},
	}
	runCtx, cancel := context.WithCancel(ctx)
	o.cancel = cancel
	defer cancel()

	o.emit(event.SwarmStarted, nil, "", "", "")

	plan := o.plan(runCtx, objective)
	o.state.Status = StatusRunning
	o.persist()
	for _, task := range plan.Tasks {
		o.state.Tasks[task.ID] = task
		o.emit(event.SwarmTaskCreated, task, "", "", "")
	}

	o.schedule(runCtx, plan)

	if runCtx.Err() == context.Canceled {
		o.state.Status = StatusCancelled
		o.state.FinishedAt = o.Now()
		o.persist()
		o.emit(event.SwarmCancelled, nil, "", "", "cancelled")
		return o.state, context.Canceled
	}

	o.state.Status = StatusIntegrating
	o.persist()
	o.emit(event.SwarmMergeStarted, nil, "", "", "")
	o.state.Result = o.integrate()
	o.state.UpdatedAt = o.Now()
	o.persist()
	o.emit(event.SwarmMergeCompleted, nil, "", "", o.state.Result)

	if o.anyFailed() {
		o.state.Status = StatusFailed
		o.state.FinishedAt = o.Now()
		o.persist()
		o.emit(event.SwarmFailed, nil, "", "", o.state.Result)
		return o.state, nil
	}

	o.emit(event.SwarmVerificationStarted, nil, "", "", "")
	ok, reason := o.verifyIntegrated()
	o.emit(event.SwarmVerificationCompleted, nil, "", "", reason)
	if !ok {
		o.state.Status = StatusFailed
		o.state.Failed = true
		o.state.Result = "verification failed: " + reason
		o.state.FinishedAt = o.Now()
		o.persist()
		o.emit(event.SwarmFailed, nil, "", "", o.state.Result)
		return o.state, nil
	}

	o.state.Status = StatusDone
	o.state.Verified = true
	o.state.FinishedAt = o.Now()
	o.persist()
	o.emit(event.SwarmCompleted, nil, "", "", o.state.Result)
	return o.state, nil
}

// Cancel stops the whole swarm (and every running worker) without corrupting
// persistent project/session state.
func (o *Orchestrator) Cancel() {
	if o == nil {
		return
	}
	if o.cancel != nil {
		o.cancel()
	}
}

// Snapshot returns a deep copy of the current authoritative swarm state, safe
// for concurrent callers (HTTP/APK readers).
func (o *Orchestrator) Snapshot() *SwarmState {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state == nil {
		return nil
	}
	cp := *o.state
	cp.Tasks = make(map[string]*Task, len(o.state.Tasks))
	for id, t := range o.state.Tasks {
		tt := *t
		cp.Tasks[id] = &tt
	}
	cp.Findings = append([]Finding(nil), o.state.Findings...)
	cp.Failures = append([]TaskFailure(nil), o.state.Failures...)
	cp.Budget.Providers = map[string]float64{}
	for k, v := range o.state.Budget.Providers {
		cp.Budget.Providers[k] = v
	}
	return &cp
}

// plan decides decomposition for the objective. A planner failure must never
// kill the swarm: it falls back to a single bounded task.
func (o *Orchestrator) plan(ctx context.Context, objective string) *Plan {
	p := o.planner
	if p == nil {
		p = deterministicPlanner{}
	}
	plan, err := p.Plan(ctx, o, objective)
	if err != nil || plan == nil || len(plan.Tasks) == 0 {
		plan = o.singleTaskPlan(objective)
	}
	return plan
}

func (o *Orchestrator) anyFailed() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, t := range o.state.Tasks {
		if t.Status == TaskFailed {
			return true
		}
	}
	return false
}

func (o *Orchestrator) emit(kind event.Kind, task *Task, workerID, provider, summary string) {
	if o.sink == nil {
		return
	}
	ev := event.SwarmEvent{SwarmID: o.state.ID, SubEvent: swarmSubEvent(kind), Summary: summary, Provider: provider, WorkerID: workerID}
	if task != nil {
		ev.TaskID = task.ID
		ev.Status = string(task.Status)
		ev.ModelRef = task.Model
		if task.Failure != nil {
			ev.Failure = string(task.Failure.Class)
		}
	} else {
		ev.Status = string(o.state.Status)
	}
	o.sink.Emit(event.Event{Kind: kind, Swarm: &ev})
}

func swarmSubEvent(kind event.Kind) string {
	switch kind {
	case event.SwarmStarted:
		return "swarm.started"
	case event.SwarmTaskCreated:
		return "swarm.task.created"
	case event.SwarmTaskAssigned:
		return "swarm.task.assigned"
	case event.SwarmAgentStarted:
		return "swarm.agent.started"
	case event.SwarmAgentReasoning:
		return "swarm.agent.reasoning"
	case event.SwarmAgentToolDispatch:
		return "swarm.agent.tool_dispatch"
	case event.SwarmAgentToolResult:
		return "swarm.agent.tool_result"
	case event.SwarmAgentCompleted:
		return "swarm.agent.completed"
	case event.SwarmAgentFailed:
		return "swarm.agent.failed"
	case event.SwarmTaskCompleted:
		return "swarm.task.completed"
	case event.SwarmTaskFailed:
		return "swarm.task.failed"
	case event.SwarmMergeStarted:
		return "swarm.merge.started"
	case event.SwarmMergeCompleted:
		return "swarm.merge.completed"
	case event.SwarmVerificationStarted:
		return "swarm.verification.started"
	case event.SwarmVerificationCompleted:
		return "swarm.verification.completed"
	case event.SwarmCompleted:
		return "swarm.completed"
	case event.SwarmFailed:
		return "swarm.failed"
	case event.SwarmCancelled:
		return "swarm.cancelled"
	}
	return string(rune(kind))
}

// schedule runs all tasks with bounded parallelism, respecting dependencies,
// worker count, budget, and cancellation. Retryable worker failures return a
// task to the ready pool for another attempt (bounded by MaxAttempts).
func (o *Orchestrator) schedule(ctx context.Context, plan *Plan) {
	o.mu.Lock()
	remaining := map[string]*Task{}
	for _, t := range plan.Tasks {
		remaining[t.ID] = t
	}
	o.mu.Unlock()

	for {
		if ctx.Err() != nil || o.budgetExhausted() {
			break
		}
		o.mu.Lock()
		for id, t := range remaining {
			if t.Status == TaskCancelled {
				delete(remaining, id)
			}
		}
		var runnable []*Task
		for _, t := range remaining {
			if (t.Status == TaskPending || t.Status == TaskReady) && depsSatisfied(remaining, t) {
				runnable = append(runnable, t)
			}
		}
		sort.Slice(runnable, func(i, j int) bool { return runnable[i].ID < runnable[j].ID })
		o.mu.Unlock()

		if len(runnable) == 0 {
			o.markDeadlocked(remaining)
			break
		}
		toRun := runnable
		if len(toRun) > o.Limits.MaxWorkers {
			toRun = toRun[:o.Limits.MaxWorkers]
		}
		for _, t := range toRun {
			o.mu.Lock()
			if t.Status == TaskPending || t.Status == TaskReady {
				t.Status = TaskRunning
			}
			o.mu.Unlock()
		}
		var wg sync.WaitGroup
		for _, t := range toRun {
			wg.Add(1)
			go func(task *Task) {
				defer wg.Done()
				o.runTask(ctx, task)
			}(t)
		}
		wg.Wait()
	}

	o.mu.Lock()
	for _, t := range remaining {
		if t.Status == TaskPending || t.Status == TaskReady {
			t.Status = TaskCancelled
		}
	}
	o.state.UpdatedAt = o.Now()
	o.mu.Unlock()
	o.persist()
}

func depsSatisfied(all map[string]*Task, t *Task) bool {
	for _, dep := range t.Dependencies {
		depTask, ok := all[dep]
		if !ok || depTask == nil {
			continue
		}
		if depTask.Status != TaskSucceeded {
			return false
		}
	}
	return true
}

func (o *Orchestrator) markDeadlocked(remaining map[string]*Task) {
	o.mu.Lock()
	defer o.mu.Unlock()
	blocked := false
	for _, t := range remaining {
		if t.Status != TaskPending && t.Status != TaskReady {
			continue
		}
		// Not runnable: either a dependency failed or the dependency itself is
		// not runnable yet. Surface a dependency failure so the swarm can end.
		for _, dep := range t.Dependencies {
			if depTask, ok := remaining[dep]; ok && depTask != nil && depTask.Status == TaskFailed {
				t.Status = TaskFailed
				t.Failure = &TaskFailure{Class: FailureDependency, Message: "dependency failed", At: o.Now()}
				blocked = true
				break
			}
		}
	}
	if blocked {
		return
	}
	// No runnable task and nothing failed: everything still pending is cancelled
	// (the swarm was cancelled or the budget stopped mid-plan).
	for _, t := range remaining {
		if t.Status == TaskPending || t.Status == TaskReady {
			t.Status = TaskCancelled
		}
	}
}

func (o *Orchestrator) budgetExhausted() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.budgetExhaustedLocked()
}

// budgetExhaustedLocked reports budget exhaustion; the caller must hold o.mu.
func (o *Orchestrator) budgetExhaustedLocked() bool {
	b := &o.state.Budget
	if o.Limits.TotalCost > 0 && b.CostSpent >= o.Limits.TotalCost {
		return true
	}
	if o.Limits.TotalTokens > 0 && b.TokensSpent >= o.Limits.TotalTokens {
		return true
	}
	return false
}

func (o *Orchestrator) runTask(ctx context.Context, task *Task) {
	o.mu.Lock()
	task.Status = TaskRunning
	task.StartedAt = o.Now()
	task.WorkerID = task.ID // one worker per task in the current runtime
	profile := o.profileFor(task.Profile)
	o.mu.Unlock()

	o.emit(event.SwarmTaskAssigned, task, task.WorkerID, "", "")
	o.emit(event.SwarmAgentStarted, task, task.WorkerID, "", "")

	outcome := runWorker(ctx, workerConfig{
		Resolver: o.Resolver,
		Registry: o.Registry,
		Profiles: o.Profiles,
		Default:  o.Default,
		Sink:     o.sink,
	}, task, profile)

	o.mu.Lock()
	defer o.mu.Unlock()
	task.Model = outcome.ModelRef
	task.Provider = outcome.Provider
	task.UpdatedAt = o.Now()
	o.state.Budget.addCost(outcome.CostSpent)
	o.state.Budget.addTokens(outcome.Tokens)
	o.state.Budget.addRequest(outcome.Provider, outcome.CostSpent)

	if outcome.Cancelled {
		task.Status = TaskCancelled
		task.Failure = &TaskFailure{Class: FailureCancelled, Message: "cancelled", At: o.Now()}
		task.FinishedAt = o.Now()
		o.persist()
		return
	}
	if outcome.Failure == nil {
		task.Result = outcome.Result
		task.Status = TaskSucceeded
		task.FinishedAt = o.Now()
		o.collectFindings(task)
		o.persist()
		o.emit(event.SwarmAgentCompleted, task, task.WorkerID, outcome.Provider, task.Result.Summary)
		o.emit(event.SwarmTaskCompleted, task, task.WorkerID, outcome.Provider, task.Result.Summary)
		return
	}
	task.Attempts++
	failure := outcome.Failure
	task.Failure = failure
	if failure.Retryable && task.Attempts < o.MaxAttempts && ctx.Err() == nil && o.nowAllowance() {
		task.Status = TaskReady
		o.persist()
		o.emit(event.SwarmAgentFailed, task, task.WorkerID, outcome.Provider, failure.Message)
		return
	}
	task.Status = TaskFailed
	task.FinishedAt = o.Now()
	o.state.Failures = append(o.state.Failures, *failure)
	o.persist()
	o.emit(event.SwarmAgentFailed, task, task.WorkerID, outcome.Provider, failure.Message)
	o.emit(event.SwarmTaskFailed, task, task.WorkerID, outcome.Provider, failure.Message)
}

func (o *Orchestrator) nowAllowance() bool {
	return !o.budgetExhaustedLocked()
}

func (o *Orchestrator) collectFindings(task *Task) {
	if task.Result == nil {
		return
	}
	for _, f := range task.Result.Findings {
		f.TaskID = task.ID
		f.Verified = true
		o.state.Findings = append(o.state.Findings, f)
	}
}

// profileFor resolves a task's profile, falling back to the default and then
// to the researcher profile.
func (o *Orchestrator) profileFor(name string) Profile {
	if p, ok := o.Profiles[name]; ok {
		return p
	}
	if p, ok := o.Profiles[o.Default]; ok {
		return p
	}
	if p, ok := o.Profiles["researcher"]; ok {
		return p
	}
	return Profile{Name: "default", Instructions: "Complete the objective.", MaxSteps: 20, RequiredEvidence: []EvidenceKind{EvidenceProvider}}
}

// persist writes the structured swarm state under the Reasonix home. Raw
// transcripts, prompts, and hidden reasoning are never persisted.
func (o *Orchestrator) persist() {
	if o == nil || o.state == nil {
		return
	}
	// Persistence is best-effort here; the serve layer owns the durable store.
	if o.persister != nil {
		_ = o.persister(o.state)
	}
}
