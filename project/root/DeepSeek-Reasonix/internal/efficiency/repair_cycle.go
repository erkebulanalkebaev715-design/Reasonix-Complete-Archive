package efficiency

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// VerificationReceipt is host-owned evidence for one repair attempt. A model's
// prose is never a pass condition: at least one required check must have run,
// every required check must have passed, and an observed build must exit 0.
type VerificationReceipt struct {
	RequiredChecks int  `json:"requiredChecks"`
	ChecksPassed   int  `json:"checksPassed"`
	ChecksFailed   int  `json:"checksFailed"`
	BuildObserved  bool `json:"buildObserved"`
	BuildExitCode  int  `json:"buildExitCode"`
}

func (v VerificationReceipt) Passed() bool {
	if v.RequiredChecks <= 0 || v.ChecksFailed > 0 || v.ChecksPassed < v.RequiredChecks {
		return false
	}
	return !v.BuildObserved || v.BuildExitCode == 0
}

// RepairAttempt is the objective result of one executor strategy. Fields that
// may contain source/log content (FixHint, BuildLog) are consumed internally and
// are deliberately absent from RepairCycleSnapshot, which is APK-safe.
type RepairAttempt struct {
	StrategyID          string              `json:"strategyId"`
	FailureFingerprint  string              `json:"failureFingerprint,omitempty"`
	ResolvedFingerprint string              `json:"resolvedFingerprint,omitempty"`
	Environment         string              `json:"environment,omitempty"`
	Files               []string            `json:"files,omitempty"`
	FixHint             string              `json:"fixHint,omitempty"`
	PatchNumstat        string              `json:"patchNumstat,omitempty"`
	Regression          bool                `json:"regression,omitempty"`
	BuildLog            string              `json:"buildLog,omitempty"`
	Verification        VerificationReceipt `json:"verification"`
	EstimatedFlashKZT   float64             `json:"estimatedFlashKzt,omitempty"`
	EstimatedProKZT     float64             `json:"estimatedProKzt,omitempty"`
	Finalization        bool                `json:"finalization,omitempty"`
}

// RecoveryStatus is content-free rollback telemetry. Rollback is supplied by a
// host adapter so the core never reimplements Reasonix's checkpoint engine.
type RecoveryStatus struct {
	Requested bool   `json:"requested"`
	Succeeded bool   `json:"succeeded"`
	Reason    string `json:"reason,omitempty"`
	Error     string `json:"error,omitempty"`
}

// RepairCycleSnapshot is safe to expose through /mod/*. It contains decisions,
// counters and fingerprints only; never prompts, code, raw logs, or fix hints.
type RepairCycleSnapshot struct {
	State                  string              `json:"state"`
	Attempts               int                 `json:"attempts"`
	LastRoute              RouteDecision       `json:"lastRoute"`
	LastFailureFingerprint string              `json:"lastFailureFingerprint,omitempty"`
	LastStrategyID         string              `json:"lastStrategyId,omitempty"`
	Verification           VerificationReceipt `json:"verification"`
	Patch                  PatchReport         `json:"patch"`
	CacheHit               bool                `json:"cacheHit"`
	CacheScore             float64             `json:"cacheScore,omitempty"`
	Cache                  FailureCacheStats   `json:"cache"`
	Recovery               RecoveryStatus      `json:"recovery"`
	Rollbacks              int                 `json:"rollbacks"`
	RollbackFailures       int                 `json:"rollbackFailures"`
	UpdatedAtUnixMS        int64               `json:"updatedAtUnixMs"`
}

// RepairCycleResult is the host-internal decision. CachedHint and LogSummary may
// contain task content and therefore must not be forwarded to the APK event bus.
type RepairCycleResult struct {
	Snapshot   RepairCycleSnapshot
	CachedHint string
	LogSummary LogSummary
}

type RollbackFunc func(reason string) error

// RepairCycle joins the v0.3 policy modules into one host-owned loop. It still
// does not call providers itself: provider transport/model switching remains a
// controller concern. This layer decides what class of work is allowed next and
// guarantees that completion/failure-memory require external verification.
type RepairCycle struct {
	mu sync.Mutex

	router      *Escalator
	budget      *Governor
	cache       *FailureCache
	patchPolicy PatchPolicy
	logConfig   LogReduceConfig
	rollback    RollbackFunc
	snap        RepairCycleSnapshot
	lastFailure string
}

func NewRepairCycle(router *Escalator, budget *Governor, cache *FailureCache) *RepairCycle {
	if router == nil {
		router = NewEscalator()
	}
	return &RepairCycle{
		router:      router,
		budget:      budget,
		cache:       cache,
		patchPolicy: DefaultPatchPolicy(),
		logConfig:   DefaultLogReduceConfig(),
		snap: RepairCycleSnapshot{
			State:     "idle",
			LastRoute: router.Snapshot(),
		},
	}
}

func (c *RepairCycle) SetRollback(fn RollbackFunc) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.rollback = fn
	c.mu.Unlock()
}

func (c *RepairCycle) SetPatchPolicy(p PatchPolicy) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.patchPolicy = p
	c.mu.Unlock()
}

func (c *RepairCycle) Snapshot() RepairCycleSnapshot {
	if c == nil {
		return RepairCycleSnapshot{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.snap
	if c.cache != nil {
		out.Cache = c.cache.Stats()
	}
	return out
}

func (c *RepairCycle) Reset() RepairCycleSnapshot {
	if c == nil {
		return RepairCycleSnapshot{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastFailure = ""
	c.snap = RepairCycleSnapshot{State: "idle"}
	c.snap.LastRoute = c.router.Reset()
	if c.cache != nil {
		c.snap.Cache = c.cache.Stats()
	}
	c.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	return c.snap
}

func (c *RepairCycle) Report(a RepairAttempt) (RepairCycleResult, error) {
	if c == nil {
		return RepairCycleResult{}, fmt.Errorf("repair cycle is nil")
	}
	if err := ValidateRouteInput(RouteInput{EstimatedFlashKZT: a.EstimatedFlashKZT, EstimatedProKZT: a.EstimatedProKZT}); err != nil {
		return RepairCycleResult{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.snap.Attempts++
	c.snap.State = "verifying"
	c.snap.LastStrategyID = strings.TrimSpace(a.StrategyID)
	c.snap.Verification = a.Verification
	c.snap.CacheHit = false
	c.snap.CacheScore = 0
	c.snap.Recovery = RecoveryStatus{}

	patch := PatchReport{Allowed: true}
	if strings.TrimSpace(a.PatchNumstat) != "" {
		var err error
		patch, err = CheckNumstat(a.PatchNumstat, c.patchPolicy)
		if err != nil {
			return RepairCycleResult{}, err
		}
	}
	c.snap.Patch = patch
	logSummary := ReduceBuildLog(a.BuildLog, c.logConfig)

	failure := strings.TrimSpace(a.FailureFingerprint)
	if failure != "" {
		c.lastFailure = failure
		c.snap.LastFailureFingerprint = failure
	}

	needRollback := a.Regression || !patch.Allowed
	if needRollback {
		reason := "verification regression"
		if !patch.Allowed {
			reason = patch.Reason
		}
		c.snap.Recovery = RecoveryStatus{Requested: true, Reason: reason}
		if c.rollback == nil {
			c.snap.Recovery.Error = "rollback adapter unavailable"
			c.snap.RollbackFailures++
		} else if err := c.rollback(reason); err != nil {
			c.snap.Recovery.Error = err.Error()
			c.snap.RollbackFailures++
		} else {
			c.snap.Recovery.Succeeded = true
			c.snap.Rollbacks++
		}
	}

	verified := a.Verification.Passed() && patch.Allowed && !a.Regression
	if verified {
		resolved := strings.TrimSpace(a.ResolvedFingerprint)
		if resolved == "" {
			resolved = c.lastFailure
		}
		if resolved != "" && strings.TrimSpace(a.FixHint) != "" && c.cache != nil {
			if err := c.cache.PutVerified(FailureRecord{
				Fingerprint: resolved,
				Environment: strings.TrimSpace(a.Environment),
				Files:       a.Files,
				FixHint:     a.FixHint,
				Verified:    true,
			}); err != nil {
				return RepairCycleResult{}, err
			}
		}
		c.snap.LastRoute = c.router.Decide(RouteInput{Passed: true, Finalization: a.Finalization}, c.budget)
		c.snap.State = "done"
		c.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
		if c.cache != nil {
			c.snap.Cache = c.cache.Stats()
		}
		return RepairCycleResult{Snapshot: c.snap, LogSummary: logSummary}, nil
	}

	cachedHint := ""
	if failure != "" && c.cache != nil {
		if rec, score, ok := c.cache.Lookup(failure, a.Environment, a.Files); ok {
			c.snap.CacheHit = true
			c.snap.CacheScore = score
			cachedHint = rec.FixHint
		}
	}

	c.snap.LastRoute = c.router.Decide(RouteInput{
		Passed:             false,
		FailureFingerprint: failure,
		StrategyID:         a.StrategyID,
		EstimatedFlashKZT:  a.EstimatedFlashKZT,
		EstimatedProKZT:    a.EstimatedProKZT,
		Finalization:       a.Finalization,
	}, c.budget)
	c.snap.State = stateForRoute(c.snap.LastRoute.Action)
	if c.snap.Recovery.Requested && !c.snap.Recovery.Succeeded {
		c.snap.State = "rollback_blocked"
	}
	c.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	if c.cache != nil {
		c.snap.Cache = c.cache.Stats()
	}
	return RepairCycleResult{Snapshot: c.snap, CachedHint: cachedHint, LogSummary: logSummary}, nil
}

func stateForRoute(a RouteAction) string {
	switch a {
	case RouteRetryFlash:
		return "retrying_flash"
	case RouteDiagnosePro:
		return "diagnosing_pro"
	case RouteExecuteFlash:
		return "executing_flash"
	case RouteStopBudget:
		return "budget_blocked"
	case RouteStopNoProgress:
		return "stalled"
	case RouteFinalize:
		return "done"
	default:
		return "working"
	}
}
