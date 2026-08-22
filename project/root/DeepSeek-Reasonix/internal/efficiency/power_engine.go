package efficiency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PowerAttempt is one externally-observed repair attempt plus the concrete
// model currently bound to the Reasonix controller. The attempt itself carries
// only host evidence; model prose is never a success signal.
type PowerAttempt struct {
	RepairAttempt
	CurrentModelRef string `json:"currentModelRef,omitempty"`
}

// PowerSnapshot is the single APK-safe state for the Balance power/economy
// loop. It intentionally contains only counters, policy decisions and model
// refs; prompts, code, logs and cached fix text stay host-internal.
type PowerSnapshot struct {
	Cycle           RepairCycleSnapshot `json:"cycle"`
	Execution       ExecutionSnapshot   `json:"execution"`
	Budget          BudgetSnapshot      `json:"budget"`
	NextAction      RouteAction         `json:"nextAction,omitempty"`
	Terminal        bool                `json:"terminal"`
	Blocked         bool                `json:"blocked"`
	Reason          string              `json:"reason,omitempty"`
	AutomaticSwitch bool                `json:"automaticSwitch"`
	UpdatedAtUnixMS int64               `json:"updatedAtUnixMs"`
}

// PowerResult keeps content-bearing repair details private while exposing one
// coherent snapshot to the host/APK.
type PowerResult struct {
	Snapshot PowerSnapshot
	Repair   RepairCycleResult
}

// PowerEngine is the single deterministic coordinator for the previously
// separate RepairCycle and ExecutionRouter. It does not call a provider
// directly; model changes still go through Reasonix's native switchModel
// adapter owned by ExecutionRouter.
type PowerEngine struct {
	mu sync.Mutex

	cycle  *RepairCycle
	exec   *ExecutionRouter
	budget *Governor
	snap   PowerSnapshot
}

func NewPowerEngine(cycle *RepairCycle, exec *ExecutionRouter, budget *Governor) *PowerEngine {
	p := &PowerEngine{cycle: cycle, exec: exec, budget: budget}
	p.snap = p.snapshotLocked("initialized", false)
	return p
}

func (p *PowerEngine) Snapshot() PowerSnapshot {
	if p == nil {
		return PowerSnapshot{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snap
}

func (p *PowerEngine) Reset(currentModelRef string) PowerSnapshot {
	if p == nil {
		return PowerSnapshot{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cycle != nil {
		p.cycle.Reset()
	}
	if p.exec != nil {
		p.exec.Reset(currentModelRef)
	}
	p.snap = p.snapshotLocked("reset", false)
	return p.snap
}

// Handle is the only Balance path that should combine verification, routing,
// budget admission and model switching. A route is decided first, then the
// concrete model switch is admitted immediately against the latest KZT budget.
func (p *PowerEngine) Handle(ctx context.Context, in PowerAttempt) (PowerResult, error) {
	if p == nil || p.cycle == nil || p.exec == nil {
		return PowerResult{}, fmt.Errorf("power engine is not fully configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	repair, err := p.cycle.Report(in.RepairAttempt)
	if err != nil {
		p.snap = p.snapshotLocked(err.Error(), true)
		return PowerResult{Snapshot: p.snap, Repair: repair}, err
	}

	execSnap, execErr := p.exec.Apply(ctx, ExecutionRequest{
		Decision:          repair.Snapshot.LastRoute,
		CurrentModelRef:   in.CurrentModelRef,
		EstimatedFlashKZT: in.EstimatedFlashKZT,
		EstimatedProKZT:   in.EstimatedProKZT,
		Finalization:      in.Finalization,
	})

	p.snap = p.snapshotLocked(repair.Snapshot.LastRoute.Reason, execErr != nil || execSnap.Blocked)
	p.snap.Cycle = repair.Snapshot
	p.snap.Execution = execSnap
	p.snap.NextAction = repair.Snapshot.LastRoute.Action
	p.snap.Terminal = powerRouteTerminal(repair.Snapshot.LastRoute.Action)
	p.snap.AutomaticSwitch = execSnap.Enabled
	p.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	if execErr != nil {
		p.snap.Reason = execSnap.Reason
		return PowerResult{Snapshot: p.snap, Repair: repair}, execErr
	}
	return PowerResult{Snapshot: p.snap, Repair: repair}, nil
}

func (p *PowerEngine) snapshotLocked(reason string, blocked bool) PowerSnapshot {
	out := PowerSnapshot{Reason: reason, Blocked: blocked, UpdatedAtUnixMS: time.Now().UnixMilli()}
	if p.cycle != nil {
		out.Cycle = p.cycle.Snapshot()
		out.NextAction = out.Cycle.LastRoute.Action
		out.Terminal = powerRouteTerminal(out.NextAction)
	}
	if p.exec != nil {
		out.Execution = p.exec.Snapshot()
		out.AutomaticSwitch = out.Execution.Enabled
	}
	if p.budget != nil {
		out.Budget = p.budget.Snapshot()
	}
	return out
}

func powerRouteTerminal(a RouteAction) bool {
	switch a {
	case RouteFinalize, RouteStopBudget, RouteStopNoProgress:
		return true
	default:
		return false
	}
}
