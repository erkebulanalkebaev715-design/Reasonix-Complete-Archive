package efficiency

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// RouteAction is a host-owned next-step decision. The model never chooses its
// own escalation tier; callers feed objective failure/progress facts here.
type RouteAction string

const (
	RouteContinueFlash  RouteAction = "continue_flash"
	RouteRetryFlash     RouteAction = "retry_flash_alternative"
	RouteDiagnosePro    RouteAction = "diagnose_pro"
	RouteExecuteFlash   RouteAction = "execute_flash_after_pro"
	RouteStopBudget     RouteAction = "stop_budget"
	RouteStopNoProgress RouteAction = "stop_no_progress"
	RouteFinalize       RouteAction = "finalize"
)

// RouteInput contains only objective host facts. StrategyID should change when
// the executor genuinely changes its approach; FailureFingerprint should remain
// stable for materially the same failure/root cause.
type RouteInput struct {
	Passed             bool    `json:"passed"`
	FailureFingerprint string  `json:"failureFingerprint,omitempty"`
	StrategyID         string  `json:"strategyId,omitempty"`
	EstimatedFlashKZT  float64 `json:"estimatedFlashKzt,omitempty"`
	EstimatedProKZT    float64 `json:"estimatedProKzt,omitempty"`
	Finalization       bool    `json:"finalization,omitempty"`
}

// RouteDecision is safe to expose to the APK: it contains no prompts, code or
// provider reasoning, only policy state and the chosen tier.
type RouteDecision struct {
	Action             RouteAction `json:"action"`
	Reason             string      `json:"reason"`
	FlashAttempts      int         `json:"flashAttempts"`
	DistinctStrategies int         `json:"distinctStrategies"`
	SameFailureStreak  int         `json:"sameFailureStreak"`
	ProDiagnoses       int         `json:"proDiagnoses"`
	UpdatedAtUnixMS    int64       `json:"updatedAtUnixMs"`
}

// Escalator implements the Balance Mod ladder:
// Flash -> alternative Flash -> short Pro diagnosis -> Flash execution.
// It deliberately does not call providers itself; that stays transport/runtime
// plumbing. This state machine is deterministic and independently testable.
type Escalator struct {
	mu sync.Mutex

	FlashAttempts     int
	ProDiagnoses      int
	SameFailureStreak int
	LastFailure       string
	strategies        map[string]struct{}
	ProDiagnosisArmed bool
	last              RouteDecision
}

func NewEscalator() *Escalator {
	return &Escalator{strategies: make(map[string]struct{})}
}

func (e *Escalator) Reset() RouteDecision {
	if e == nil {
		return RouteDecision{}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.FlashAttempts = 0
	e.ProDiagnoses = 0
	e.SameFailureStreak = 0
	e.LastFailure = ""
	e.strategies = make(map[string]struct{})
	e.ProDiagnosisArmed = false
	e.last = e.decisionLocked(RouteContinueFlash, "fresh task")
	return e.last
}

func (e *Escalator) Snapshot() RouteDecision {
	if e == nil {
		return RouteDecision{}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.last
	out.FlashAttempts = e.FlashAttempts
	out.ProDiagnoses = e.ProDiagnoses
	out.SameFailureStreak = e.SameFailureStreak
	out.DistinctStrategies = len(e.strategies)
	return out
}

// Decide consumes one completed attempt/checkpoint. Budget admission uses the
// existing Governor and therefore cannot be bypassed by model prose.
func (e *Escalator) Decide(in RouteInput, budget *Governor) RouteDecision {
	if e == nil {
		return RouteDecision{Action: RouteContinueFlash, Reason: "router disabled"}
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if in.Passed {
		e.last = e.decisionLocked(RouteFinalize, "verification passed")
		return e.last
	}

	failure := strings.TrimSpace(in.FailureFingerprint)
	strategy := strings.TrimSpace(in.StrategyID)
	if strategy != "" {
		e.strategies[strategy] = struct{}{}
	}

	if failure != "" && failure == e.LastFailure {
		e.SameFailureStreak++
	} else {
		e.LastFailure = failure
		if failure == "" {
			e.SameFailureStreak = 0
		} else {
			e.SameFailureStreak = 1
		}
	}

	// After Pro has diagnosed, the expensive tier is not allowed to keep doing
	// edits. Return immediately to Flash for execution.
	if e.ProDiagnosisArmed {
		if budget != nil {
			if ok, why := budget.CanSpend("flash", in.EstimatedFlashKZT, in.Finalization); !ok {
				e.last = e.decisionLocked(RouteStopBudget, why)
				return e.last
			}
		}
		e.ProDiagnosisArmed = false
		e.FlashAttempts++
		e.last = e.decisionLocked(RouteExecuteFlash, "Pro diagnosis consumed; execution returns to Flash")
		return e.last
	}

	// First failed Flash attempt gets one genuinely different cheap attempt.
	if e.FlashAttempts == 0 {
		if budget != nil {
			if ok, why := budget.CanSpend("flash", in.EstimatedFlashKZT, in.Finalization); !ok {
				e.last = e.decisionLocked(RouteStopBudget, why)
				return e.last
			}
		}
		e.FlashAttempts++
		e.last = e.decisionLocked(RouteRetryFlash, "first failure: try a different Flash strategy")
		return e.last
	}

	// Escalate only when a materially same failure survives >=2 distinct
	// strategies. This avoids paying Pro merely because error text repeats.
	if e.SameFailureStreak >= 2 && len(e.strategies) >= 2 {
		if budget != nil {
			if ok, why := budget.CanSpend("pro", in.EstimatedProKZT, false); !ok {
				// Budget denied Pro. One more cheap strategy is allowed when
				// possible; otherwise stop rather than loop forever.
				if len(e.strategies) < 3 {
					if ok2, _ := budget.CanSpend("flash", in.EstimatedFlashKZT, in.Finalization); ok2 {
						e.FlashAttempts++
						e.last = e.decisionLocked(RouteRetryFlash, "Pro blocked by budget; use one more Flash strategy")
						return e.last
					}
				}
				e.last = e.decisionLocked(RouteStopBudget, why)
				return e.last
			}
		}
		e.ProDiagnoses++
		e.ProDiagnosisArmed = true
		e.last = e.decisionLocked(RouteDiagnosePro, "same failure survived distinct Flash strategies")
		return e.last
	}

	// Different failure/root cause means there is still objective movement.
	// Keep the cheap tier, but cap unproductive wandering.
	if len(e.strategies) >= 4 {
		e.last = e.decisionLocked(RouteStopNoProgress, "too many distinct failed Flash strategies without a stable escalation signal")
		return e.last
	}
	if budget != nil {
		if ok, why := budget.CanSpend("flash", in.EstimatedFlashKZT, in.Finalization); !ok {
			e.last = e.decisionLocked(RouteStopBudget, why)
			return e.last
		}
	}
	e.FlashAttempts++
	e.last = e.decisionLocked(RouteRetryFlash, "failure changed; continue cheap diagnosis")
	return e.last
}

func (e *Escalator) decisionLocked(action RouteAction, reason string) RouteDecision {
	return RouteDecision{
		Action:             action,
		Reason:             reason,
		FlashAttempts:      e.FlashAttempts,
		DistinctStrategies: len(e.strategies),
		SameFailureStreak:  e.SameFailureStreak,
		ProDiagnoses:       e.ProDiagnoses,
		UpdatedAtUnixMS:    time.Now().UnixMilli(),
	}
}

func ValidateRouteInput(in RouteInput) error {
	if in.EstimatedFlashKZT < 0 || in.EstimatedProKZT < 0 {
		return fmt.Errorf("estimated costs must be non-negative")
	}
	return nil
}
