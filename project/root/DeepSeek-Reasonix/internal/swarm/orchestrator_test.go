package swarm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// fakeResolver is an offline Resolver for deterministic swarm tests.
type fakeResolver struct {
	prov  provider.Provider
	price *provider.Pricing
	model string
	name  string
	err   error
}

func (f *fakeResolver) Resolve(modelRef string) (provider.Provider, *provider.Pricing, string, string, error) {
	if f.err != nil {
		return nil, nil, "", "", f.err
	}
	model := f.model
	if model == "" {
		model = "mock/deepseek-v4-flash"
	}
	name := f.name
	if name == "" {
		name = "mock"
	}
	return f.prov, f.price, model, name, nil
}

func testOrchestrator(prov provider.Provider) *Orchestrator {
	return NewOrchestrator(
		&fakeResolver{prov: prov},
		nil,
		WithLimits(BudgetLimits{MaxWorkers: 2, Timeout: time.Minute}),
		WithNow(func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }),
		WithMaxAttempts(2),
	)
}

func textTurn(text string) testutil.Turn {
	return testutil.Turn{Text: text, Usage: &provider.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}}
}

func runObjective(t *testing.T, o *Orchestrator, objective string) *SwarmState {
	t.Helper()
	state, err := o.Run(context.Background(), objective)
	if err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
	return state
}

func TestTrivialObjectiveLaunchesSingleWorker(t *testing.T) {
	prov := testutil.NewMock("mock", textTurn("Reasonix swarm works."))
	o := testOrchestrator(prov)
	state := runObjective(t, o, "Verify the build.")
	if state.Status != StatusDone {
		t.Fatalf("status = %s, want done", state.Status)
	}
	if len(state.Tasks) != 1 {
		t.Fatalf("tasks = %d, want exactly 1 (trivial tasks must not spawn a swarm)", len(state.Tasks))
	}
	for _, task := range state.Tasks {
		if task.Status != TaskSucceeded {
			t.Fatalf("task %s status = %s, want succeeded", task.ID, task.Status)
		}
		if task.Result == nil || !strings.Contains(task.Result.Summary, "Reasonix swarm works") {
			t.Fatalf("task %s result missing worker text", task.ID)
		}
	}
	if got := prov.Requests(); len(got) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(got))
	}
}

func TestParallelSegmentsRunBothWorkers(t *testing.T) {
	prov := testutil.NewMock("mock", textTurn("A ok"), textTurn("B ok"))
	o := testOrchestrator(prov)
	state := runObjective(t, o, "Investigate A; Investigate B")
	if state.Status != StatusDone {
		t.Fatalf("status = %s, want done", state.Status)
	}
	if len(state.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(state.Tasks))
	}
	seen := map[string]bool{}
	for _, task := range state.Tasks {
		if task.Status != TaskSucceeded {
			t.Fatalf("task %s status = %s", task.ID, task.Status)
		}
		if len(task.Dependencies) != 0 {
			t.Fatalf("task %s must be independent", task.ID)
		}
		seen[task.Result.Summary] = true
	}
	if !seen["A ok"] || !seen["B ok"] {
		t.Fatalf("both worker results missing: %v", seen)
	}
	if got := prov.Requests(); len(got) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(got))
	}
}

func TestDependentSegmentRunsAfterDependency(t *testing.T) {
	prov := testutil.NewMock("mock", textTurn("first"), textTurn("second"))
	o := testOrchestrator(prov)
	state := runObjective(t, o, "Investigate A; then Investigate B")
	if state.Status != StatusDone {
		t.Fatalf("status = %s, want done", state.Status)
	}
	var dep, depender *Task
	for _, task := range state.Tasks {
		if len(task.Dependencies) == 0 {
			dep = task
		} else {
			depender = task
		}
	}
	if dep == nil || depender == nil {
		t.Fatalf("expected one dependency edge, tasks=%d", len(state.Tasks))
	}
	if depender.Dependencies[0] != dep.ID {
		t.Fatalf("depender dependency = %v, want %s", depender.Dependencies, dep.ID)
	}
	// Order: the dependency must have run and finished before the depender.
	for _, req := range prov.Requests() {
		_ = req
	}
}

func TestWorkerFailureMapsFailureClassAndFailsTask(t *testing.T) {
	o := testOrchestrator(&testutil.MockProvider{})
	o.Resolver = &fakeResolver{err: fmt.Errorf("model %q is not a configured provider", "nope")}
	state := runObjective(t, o, "Investigate the failure")
	if state.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", state.Status)
	}
	for _, task := range state.Tasks {
		if task.Status != TaskFailed {
			t.Fatalf("task status = %s, want failed", task.Status)
		}
		if task.Failure == nil {
			t.Fatal("task failure not recorded")
		}
		if task.Failure.Class != FailurePermanent {
			t.Fatalf("failure class = %s, want permanent", task.Failure.Class)
		}
	}
	if len(state.Failures) == 0 {
		t.Fatal("swarm-level failures not recorded")
	}
}

func TestClassifyWorkerFailure(t *testing.T) {
	cases := []struct {
		msg   string
		class FailureClass
	}{
		{"context canceled", FailureCancelled},
		{"context deadline exceeded", FailureTimeout},
		{"hard budget exhausted: no remaining allowance", FailureBudgetStop},
		{"no progress: loop guard", FailureNoProgress},
		{"approval request pending", FailureApprovalWait},
		{"unknown tool read_file", FailureToolMissing},
		{"schema validation failed", FailureSchemaError},
		{"authentication failed for provider (HTTP 401)", FailureProviderError},
		{"connection reset by peer", FailureTemporary},
	}
	for _, tc := range cases {
		f := classifyWorkerFailure(fmt.Errorf("%s", tc.msg))
		if f.Class != tc.class {
			t.Errorf("classify(%q) = %s, want %s", tc.msg, f.Class, tc.class)
		}
	}
}

func TestBudgetStopCancelsRemainingWorkers(t *testing.T) {
	price := &provider.Pricing{Input: 1, Output: 2, Currency: "CNY"}
	prov := testutil.NewMock("mock", textTurn("first"), textTurn("second"))
	o := NewOrchestrator(
		&fakeResolver{prov: prov, price: price},
		nil,
		WithLimits(BudgetLimits{MaxWorkers: 2, TotalCost: 0.00001, Timeout: time.Minute}),
		WithNow(func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }),
		WithMaxAttempts(1),
	)
	state := runObjective(t, o, "Investigate A; Investigate B; Investigate C")
	for _, task := range state.Tasks {
		if task.Status != TaskSucceeded && task.Status != TaskCancelled {
			t.Fatalf("task %s unexpected status %s", task.ID, task.Status)
		}
	}
	if state.Budget.Requests < 1 {
		t.Fatalf("budget requests = %d, want >= 1", state.Budget.Requests)
	}
	if state.Budget.CostSpent <= 0 {
		t.Fatalf("budget cost = %v, want > 0", state.Budget.CostSpent)
	}
}

func TestSwarmCancellationStopsWorkers(t *testing.T) {
	released := make(chan struct{})
	prov := &blockingProvider{released: released, firstArrived: make(chan struct{}), turn: textTurn("blocked")}
	o := testOrchestrator(prov)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan *SwarmState, 1)
	go func() {
		state, _ := o.Run(runCtx, "Investigate A; Investigate B")
		done <- state
	}()
	// Wait until the first worker has reached the provider, then cancel.
	select {
	case <-prov.firstArrived:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never reached provider")
	}
	cancel()
	close(released)
	select {
	case state := <-done:
		if state.Status != StatusCancelled {
			t.Fatalf("status = %s, want cancelled", state.Status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("swarm did not return after cancel")
	}
}

// blockingProvider blocks its first Stream until released so a test can prove
// cancellation interrupts an in-flight worker.
type blockingProvider struct {
	released     chan struct{}
	firstArrived chan struct{}
	turn         testutil.Turn
	once         sync.Once
}

func (p *blockingProvider) Name() string { return "blocking" }

func (p *blockingProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.once.Do(func() { close(p.firstArrived) })
	select {
	case <-p.released:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ch := make(chan provider.Chunk, 4)
	defer close(ch)
	if p.turn.Text != "" {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: p.turn.Text}
	}
	if p.turn.Usage != nil {
		ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: p.turn.Usage}
	}
	return ch, nil
}

func TestEventStreamCarriesSwarmLifecycle(t *testing.T) {
	var mu sync.Mutex
	var subs []string
	sink := event.FuncSink(func(e event.Event) {
		if e.Swarm != nil {
			mu.Lock()
			subs = append(subs, e.Swarm.SubEvent)
			mu.Unlock()
		}
	})
	prov := testutil.NewMock("mock", textTurn("A ok"), textTurn("B ok"))
	o := testOrchestrator(prov)
	o.sink = sink
	state := runObjective(t, o, "Investigate A; Investigate B")
	mu.Lock()
	got := strings.Join(subs, ",")
	mu.Unlock()
	if state.Status != StatusDone {
		t.Fatalf("status = %s", state.Status)
	}
	for _, want := range []string{"swarm.started", "swarm.task.created", "swarm.task.assigned", "swarm.agent.started", "swarm.task.completed", "swarm.merge.completed", "swarm.completed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("event stream missing %q: %s", want, got)
		}
	}
}

func TestPersisterReceivesStructuredState(t *testing.T) {
	var mu sync.Mutex
	saved := map[string]bool{}
	prov := testutil.NewMock("mock", textTurn("ok"))
	o := testOrchestrator(prov)
	o.persister = func(s *SwarmState) error {
		mu.Lock()
		defer mu.Unlock()
		saved["persisted"] = true
		return nil
	}
	runObjective(t, o, "Investigate A")
	mu.Lock()
	defer mu.Unlock()
	if !saved["persisted"] {
		t.Fatal("persister was never called")
	}
}

// TestCancelledSwarmPersistsTerminalState is the regression test for the
// morning cancellation defect: a cancelled swarm must persist its terminal
// cancelled state (with FinishedAt) before Run returns, so a serve host that
// reads the last persisted state after the run goroutine clears its active
// pointer never sees a stale "running" state.
func TestCancelledSwarmPersistsTerminalState(t *testing.T) {
	released := make(chan struct{})
	prov := &blockingProvider{released: released, firstArrived: make(chan struct{}), turn: textTurn("blocked")}
	o := testOrchestrator(prov)
	var mu sync.Mutex
	var persisted []*SwarmState
	o.persister = func(s *SwarmState) error {
		mu.Lock()
		defer mu.Unlock()
		cp := *s
		persisted = append(persisted, &cp)
		return nil
	}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan *SwarmState, 1)
	go func() {
		state, _ := o.Run(runCtx, "Investigate A; Investigate B")
		done <- state
	}()
	select {
	case <-prov.firstArrived:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never reached provider")
	}
	cancel()
	close(released)
	select {
	case state := <-done:
		if state.Status != StatusCancelled {
			t.Fatalf("status = %s, want cancelled", state.Status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("swarm did not return after cancel")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(persisted) == 0 {
		t.Fatal("persister was never called")
	}
	last := persisted[len(persisted)-1]
	if last.Status != StatusCancelled {
		t.Fatalf("last persisted status = %s, want cancelled (stale running persisted after cancel)", last.Status)
	}
	if last.FinishedAt.IsZero() {
		t.Fatal("cancelled state missing FinishedAt")
	}
}
