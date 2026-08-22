package efficiency

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ExecutionConfig maps policy tiers onto concrete Reasonix model refs. It is
// intentionally provider-agnostic: the future APK can point these refs at
// DeepSeek, another remote provider, or offline mock providers without changing
// the repair policy.
type ExecutionConfig struct {
	Enabled             bool   `json:"enabled"`
	FlashPrimaryRef     string `json:"flashPrimaryRef,omitempty"`
	FlashAlternativeRef string `json:"flashAlternativeRef,omitempty"`
	ProRef              string `json:"proRef,omitempty"`
	FlashRepairRef      string `json:"flashRepairRef,omitempty"`
}

// ExecutionMode is an APK-safe phase label. pro_diagnosis is deliberately a
// separate mode so later tool-permission wiring can make it read-only without
// changing the router contract.
type ExecutionMode string

const (
	ExecutionDisabled         ExecutionMode = "disabled"
	ExecutionFlashPrimary     ExecutionMode = "flash_primary"
	ExecutionFlashAlternative ExecutionMode = "flash_alternative"
	ExecutionProDiagnosis     ExecutionMode = "pro_diagnosis"
	ExecutionFlashRepair      ExecutionMode = "flash_repair"
	ExecutionFinished         ExecutionMode = "finished"
	ExecutionStopped          ExecutionMode = "stopped"
)

// ExecutionRequest is the host-owned admission record for one route decision.
// Estimated prices are checked again immediately before a model switch so a
// stale route cannot bypass a budget update.
type ExecutionRequest struct {
	Decision          RouteDecision `json:"decision"`
	CurrentModelRef   string        `json:"currentModelRef,omitempty"`
	EstimatedFlashKZT float64       `json:"estimatedFlashKzt,omitempty"`
	EstimatedProKZT   float64       `json:"estimatedProKzt,omitempty"`
	Finalization      bool          `json:"finalization,omitempty"`
}

// ExecutionSnapshot is safe to expose through /mod/*: it contains no prompt,
// source, credentials, raw provider response, or hidden reasoning.
type ExecutionSnapshot struct {
	Configured      bool            `json:"configured"`
	Enabled         bool            `json:"enabled"`
	Mode            ExecutionMode   `json:"mode"`
	CurrentModelRef string          `json:"currentModelRef,omitempty"`
	TargetModelRef  string          `json:"targetModelRef,omitempty"`
	LastAction      RouteAction     `json:"lastAction,omitempty"`
	Switches        int             `json:"switches"`
	Blocked         bool            `json:"blocked"`
	Reason          string          `json:"reason,omitempty"`
	DiagnosisOnly   bool            `json:"diagnosisOnly"`
	UpdatedAtUnixMS int64           `json:"updatedAtUnixMs"`
	Config          ExecutionConfig `json:"config"`
}

// ModelSwitchFunc is supplied by the host (reasonix serve uses its native
// switchModel path). Keeping the switch behind an adapter makes the policy
// independently testable and prevents a second controller/session engine.
type ModelSwitchFunc func(context.Context, string) error

// ExecutionModeGuardFunc lets the host install a fail-closed policy overlay
// before a route changes execution mode/model. The returned undo function is
// called if the subsequent model switch fails, restoring the previous policy.
// A nil undo is allowed.
type ExecutionModeGuardFunc func(context.Context, ExecutionMode, string) (undo func(), err error)

// ExecutionRouter converts deterministic policy decisions into concrete model
// selections. It never decides whether Pro is deserved — Escalator owns that.
// Its job is fail-closed admission + switching + APK-visible state.
type ExecutionRouter struct {
	mu sync.Mutex

	cfg      ExecutionConfig
	budget   *Governor
	switcher ModelSwitchFunc
	guard    ExecutionModeGuardFunc
	snap     ExecutionSnapshot
}

func NewExecutionRouter(budget *Governor) *ExecutionRouter {
	r := &ExecutionRouter{budget: budget}
	r.snap = ExecutionSnapshot{Mode: ExecutionDisabled, Config: ExecutionConfig{}}
	return r
}

func normalizeExecutionConfig(c ExecutionConfig) ExecutionConfig {
	c.FlashPrimaryRef = strings.TrimSpace(c.FlashPrimaryRef)
	c.FlashAlternativeRef = strings.TrimSpace(c.FlashAlternativeRef)
	c.ProRef = strings.TrimSpace(c.ProRef)
	c.FlashRepairRef = strings.TrimSpace(c.FlashRepairRef)
	if c.FlashAlternativeRef == "" {
		c.FlashAlternativeRef = c.FlashPrimaryRef
	}
	if c.FlashRepairRef == "" {
		c.FlashRepairRef = c.FlashPrimaryRef
	}
	return c
}

func ValidateExecutionConfig(c ExecutionConfig) error {
	c = normalizeExecutionConfig(c)
	if !c.Enabled {
		return nil
	}
	if c.FlashPrimaryRef == "" {
		return fmt.Errorf("flashPrimaryRef is required when execution routing is enabled")
	}
	if c.ProRef == "" {
		return fmt.Errorf("proRef is required when execution routing is enabled")
	}
	return nil
}

func (r *ExecutionRouter) SetModeGuard(fn ExecutionModeGuardFunc) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.guard = fn
	r.mu.Unlock()
}

func (r *ExecutionRouter) SetSwitcher(fn ModelSwitchFunc) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.switcher = fn
	r.mu.Unlock()
}

func (r *ExecutionRouter) Configure(c ExecutionConfig, currentRef string) (ExecutionSnapshot, error) {
	if r == nil {
		return ExecutionSnapshot{}, fmt.Errorf("execution router is nil")
	}
	c = normalizeExecutionConfig(c)
	if err := ValidateExecutionConfig(c); err != nil {
		return ExecutionSnapshot{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = c
	r.snap = ExecutionSnapshot{
		Configured:      true,
		Enabled:         c.Enabled,
		Mode:            ExecutionDisabled,
		CurrentModelRef: strings.TrimSpace(currentRef),
		Config:          c,
		UpdatedAtUnixMS: time.Now().UnixMilli(),
	}
	if c.Enabled {
		r.snap.Mode = ExecutionFlashPrimary
		r.snap.TargetModelRef = c.FlashPrimaryRef
		r.snap.Reason = "execution routing configured"
	} else {
		r.snap.Reason = "execution routing disabled"
	}
	return r.snap, nil
}

func (r *ExecutionRouter) Reset(currentRef string) ExecutionSnapshot {
	if r == nil {
		return ExecutionSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.cfg
	r.snap = ExecutionSnapshot{
		Configured:      r.snap.Configured,
		Enabled:         c.Enabled,
		Mode:            ExecutionDisabled,
		CurrentModelRef: strings.TrimSpace(currentRef),
		Config:          c,
		UpdatedAtUnixMS: time.Now().UnixMilli(),
	}
	if c.Enabled {
		r.snap.Mode = ExecutionFlashPrimary
		r.snap.TargetModelRef = c.FlashPrimaryRef
		r.snap.Reason = "execution state reset"
	}
	return r.snap
}

func (r *ExecutionRouter) ObserveCurrentModel(ref string) ExecutionSnapshot {
	if r == nil {
		return ExecutionSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snap.CurrentModelRef = strings.TrimSpace(ref)
	r.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	return r.snap
}

func (r *ExecutionRouter) Snapshot() ExecutionSnapshot {
	if r == nil {
		return ExecutionSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snap
}

// Apply maps one already-decided route to a concrete model ref and, when
// needed, asks the host to switch. Stop/finalize decisions never trigger a
// provider call. A second budget check happens here immediately before switch.
func (r *ExecutionRouter) Apply(ctx context.Context, in ExecutionRequest) (ExecutionSnapshot, error) {
	if r == nil {
		return ExecutionSnapshot{}, fmt.Errorf("execution router is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.Lock()
	cfg := r.cfg
	switcher := r.switcher
	guard := r.guard
	priorSwitches := r.snap.Switches
	configured := r.snap.Configured
	r.mu.Unlock()

	current := strings.TrimSpace(in.CurrentModelRef)
	if !configured || !cfg.Enabled {
		snap := ExecutionSnapshot{
			Configured: configured, Enabled: false, Mode: ExecutionDisabled,
			CurrentModelRef: current, LastAction: in.Decision.Action,
			Reason: "execution routing disabled", Config: cfg,
			UpdatedAtUnixMS: time.Now().UnixMilli(), Switches: priorSwitches,
		}
		r.store(snap)
		return snap, nil
	}

	mode, target, tier, diagnosisOnly, terminal, err := selectExecutionTarget(cfg, in.Decision)
	if err != nil {
		snap := ExecutionSnapshot{
			Configured: true, Enabled: true, Mode: ExecutionStopped,
			CurrentModelRef: current, LastAction: in.Decision.Action, Blocked: true,
			Reason: err.Error(), Config: cfg, UpdatedAtUnixMS: time.Now().UnixMilli(), Switches: priorSwitches,
		}
		r.store(snap)
		return snap, err
	}
	if terminal {
		snap := ExecutionSnapshot{
			Configured: true, Enabled: true, Mode: mode,
			CurrentModelRef: current, LastAction: in.Decision.Action,
			Reason: in.Decision.Reason, Config: cfg, UpdatedAtUnixMS: time.Now().UnixMilli(), Switches: priorSwitches,
		}
		if guard != nil {
			if _, guardErr := guard(ctx, mode, ""); guardErr != nil {
				snap.Blocked = true
				snap.Reason = "execution mode guard failed: " + guardErr.Error()
				r.store(snap)
				return snap, guardErr
			}
		}
		r.store(snap)
		return snap, nil
	}

	estimated := in.EstimatedFlashKZT
	if tier == "pro" {
		estimated = in.EstimatedProKZT
	}
	if r.budget != nil {
		if ok, why := r.budget.CanSpend(tier, estimated, in.Finalization); !ok {
			snap := ExecutionSnapshot{
				Configured: true, Enabled: true, Mode: ExecutionStopped,
				CurrentModelRef: current, TargetModelRef: target, LastAction: in.Decision.Action,
				Blocked: true, Reason: why, DiagnosisOnly: diagnosisOnly, Config: cfg,
				UpdatedAtUnixMS: time.Now().UnixMilli(), Switches: priorSwitches,
			}
			r.store(snap)
			return snap, fmt.Errorf("execution budget blocked: %s", why)
		}
	}

	snap := ExecutionSnapshot{
		Configured: true, Enabled: true, Mode: mode,
		CurrentModelRef: current, TargetModelRef: target, LastAction: in.Decision.Action,
		Reason: in.Decision.Reason, DiagnosisOnly: diagnosisOnly, Config: cfg,
		UpdatedAtUnixMS: time.Now().UnixMilli(), Switches: priorSwitches,
	}
	if target == "" {
		snap.Blocked = true
		snap.Reason = "route has no configured target model"
		r.store(snap)
		return snap, fmt.Errorf("%s", snap.Reason)
	}
	var undoGuard func()
	if guard != nil {
		var guardErr error
		undoGuard, guardErr = guard(ctx, mode, target)
		if guardErr != nil {
			snap.Blocked = true
			snap.Reason = "execution mode guard failed: " + guardErr.Error()
			r.store(snap)
			return snap, guardErr
		}
	}
	if sameModelRef(current, target) {
		snap.CurrentModelRef = target
		r.store(snap)
		return snap, nil
	}
	if switcher == nil {
		if undoGuard != nil {
			undoGuard()
		}
		snap.Blocked = true
		snap.Reason = "model switch adapter unavailable"
		r.store(snap)
		return snap, fmt.Errorf("%s", snap.Reason)
	}
	if err := switcher(ctx, target); err != nil {
		if undoGuard != nil {
			undoGuard()
		}
		snap.Blocked = true
		snap.Reason = "model switch failed: " + err.Error()
		r.store(snap)
		return snap, err
	}
	snap.CurrentModelRef = target
	snap.Switches++
	snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	r.store(snap)
	return snap, nil
}

func (r *ExecutionRouter) store(s ExecutionSnapshot) {
	r.mu.Lock()
	r.snap = s
	r.mu.Unlock()
}

func sameModelRef(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func selectExecutionTarget(cfg ExecutionConfig, d RouteDecision) (ExecutionMode, string, string, bool, bool, error) {
	switch d.Action {
	case RouteContinueFlash:
		return ExecutionFlashPrimary, cfg.FlashPrimaryRef, "flash", false, false, nil
	case RouteRetryFlash:
		// Alternate the two cheap slots by the host-owned attempt counter. With
		// one actual Flash model both refs may intentionally be identical.
		if d.FlashAttempts%2 == 1 {
			return ExecutionFlashAlternative, cfg.FlashAlternativeRef, "flash", false, false, nil
		}
		return ExecutionFlashPrimary, cfg.FlashPrimaryRef, "flash", false, false, nil
	case RouteDiagnosePro:
		return ExecutionProDiagnosis, cfg.ProRef, "pro", true, false, nil
	case RouteExecuteFlash:
		return ExecutionFlashRepair, cfg.FlashRepairRef, "flash", false, false, nil
	case RouteFinalize:
		return ExecutionFinished, "", "", false, true, nil
	case RouteStopBudget, RouteStopNoProgress:
		return ExecutionStopped, "", "", false, true, nil
	default:
		return ExecutionStopped, "", "", false, false, fmt.Errorf("unsupported route action %q", d.Action)
	}
}
