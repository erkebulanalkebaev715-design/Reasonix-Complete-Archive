package serve

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"reasonix/internal/efficiency"
	"reasonix/internal/event"
)

type modPowerTurnSnapshot struct {
	Active             bool   `json:"active"`
	ModelRef           string `json:"modelRef,omitempty"`
	StrategyID         string `json:"strategyId,omitempty"`
	Mutations          int    `json:"mutations"`
	Added              int    `json:"added"`
	Removed            int    `json:"removed"`
	ChecksPassed       int    `json:"checksPassed"`
	ChecksFailed       int    `json:"checksFailed"`
	LastFailureHash    string `json:"lastFailureHash,omitempty"`
	PendingApplication bool   `json:"pendingApplication"`
	LastDecision       string `json:"lastDecision,omitempty"`
	UpdatedAtUnixMS    int64  `json:"updatedAtUnixMs"`
}

type modPowerTurnTracker struct {
	mu sync.Mutex

	snap            modPowerTurnSnapshot
	lastFailureText string
	completion      *event.CompletionSummaryInfo
	pending         *efficiency.RepairAttempt
	pendingID       string
}

func newModPowerTurnTracker() *modPowerTurnTracker { return &modPowerTurnTracker{} }

func (t *modPowerTurnTracker) Reset() modPowerTurnSnapshot {
	if t == nil {
		return modPowerTurnSnapshot{}
	}
	t.mu.Lock()
	t.snap = modPowerTurnSnapshot{UpdatedAtUnixMS: time.Now().UnixMilli()}
	t.lastFailureText = ""
	t.completion = nil
	t.pending = nil
	t.pendingID = ""
	out := t.snap
	t.mu.Unlock()
	return out
}

func (t *modPowerTurnTracker) Snapshot() modPowerTurnSnapshot {
	if t == nil {
		return modPowerTurnSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}

func (t *modPowerTurnTracker) Begin(modelRef, strategy string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.pending != nil {
		// Never silently discard a route that was derived from host evidence.
		// A client that starts another turn without applying/reviewing it loses
		// automatic power tracking for that turn, but the pending decision stays.
		t.snap.Active = false
		t.snap.LastDecision = "pending_route_requires_apply"
		t.snap.PendingApplication = true
		t.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
		t.mu.Unlock()
		return
	}
	t.snap = modPowerTurnSnapshot{Active: true, ModelRef: strings.TrimSpace(modelRef), StrategyID: strings.TrimSpace(strategy), UpdatedAtUnixMS: time.Now().UnixMilli()}
	t.lastFailureText = ""
	t.completion = nil
	t.mu.Unlock()
}

func (t *modPowerTurnTracker) Observe(e event.Event) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch e.Kind {
	case event.ToolDispatch:
		if e.Tool.Partial {
			return
		}
		if e.Tool.Added > 0 || e.Tool.Removed > 0 || (!e.Tool.ReadOnly && e.Tool.Diff != "") {
			t.snap.Mutations++
			t.snap.Added += e.Tool.Added
			t.snap.Removed += e.Tool.Removed
		}
	case event.ToolResult:
		if e.Tool.Execution != nil {
			switch strings.ToLower(strings.TrimSpace(e.Tool.Execution.Verification)) {
			case "passed":
				t.snap.ChecksPassed++
			case "failed":
				t.snap.ChecksFailed++
			}
		}
		if e.Tool.Err != "" {
			t.lastFailureText = e.Tool.Name + "|" + e.Tool.Err
			t.snap.LastFailureHash = modPowerHash(t.lastFailureText)
		} else if e.Tool.Execution != nil && strings.EqualFold(e.Tool.Execution.Verification, "failed") {
			t.lastFailureText = e.Tool.Name + "|" + e.Tool.Execution.FailurePhase + "|verification_failed"
			t.snap.LastFailureHash = modPowerHash(t.lastFailureText)
		}
	case event.CompletionSummary:
		if e.Completion != nil {
			cp := *e.Completion
			t.completion = &cp
			// CompletionSummary is the host's canonical end-of-turn count. It
			// replaces lower-level shell counts rather than double-counting.
			t.snap.ChecksPassed = cp.ChecksPassed
			t.snap.ChecksFailed = cp.ChecksFailed
		}
	}
	t.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
}

func (t *modPowerTurnTracker) Finish(e event.Event) (efficiency.RepairAttempt, bool) {
	if t == nil || e.Kind != event.TurnDone {
		return efficiency.RepairAttempt{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.snap.Active {
		return efficiency.RepairAttempt{}, false
	}

	snap := t.snap
	passed, failed := snap.ChecksPassed, snap.ChecksFailed
	if t.completion != nil {
		passed, failed = t.completion.ChecksPassed, t.completion.ChecksFailed
	}
	hasFailure := e.Err != nil || failed > 0 || e.Readiness != nil
	if snap.Mutations == 0 && passed == 0 && failed == 0 && !hasFailure {
		t.snap.Active = false
		t.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
		return efficiency.RepairAttempt{}, false
	}

	required := passed + failed
	if required == 0 {
		// A mutating turn without host verification is not considered done.
		required = 1
		if hasFailure || snap.Mutations > 0 {
			failed = 1
		}
	}

	failure := snap.LastFailureHash
	if failure == "" && (hasFailure || failed > 0) {
		parts := []string{e.Outcome, fmt.Sprint(failed), fmt.Sprint(snap.Mutations)}
		if e.Readiness != nil {
			parts = append(parts, strings.Join(e.Readiness.Missing, ","))
		}
		if e.Err != nil {
			parts = append(parts, e.Err.Error())
		}
		failure = modPowerHash(strings.Join(parts, "|"))
	}

	patch := ""
	if snap.Added > 0 || snap.Removed > 0 {
		patch = fmt.Sprintf("%d\t%d\t<turn>", snap.Added, snap.Removed)
	}
	a := efficiency.RepairAttempt{
		StrategyID:         snap.StrategyID,
		FailureFingerprint: failure,
		PatchNumstat:       patch,
		BuildLog:           t.lastFailureText,
		Verification: efficiency.VerificationReceipt{
			RequiredChecks: required,
			ChecksPassed:   passed,
			ChecksFailed:   failed,
		},
		Finalization: !hasFailure && failed == 0 && passed >= required,
	}
	if a.Finalization {
		a.ResolvedFingerprint = failure
	}
	t.snap.Active = false
	t.snap.PendingApplication = true
	t.pending = &a
	t.pendingID = newModPowerPendingID()
	t.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	return a, true
}

func (t *modPowerTurnTracker) PendingRecord() (string, efficiency.RepairAttempt, bool) {
	if t == nil {
		return "", efficiency.RepairAttempt{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending == nil {
		return "", efficiency.RepairAttempt{}, false
	}
	return t.pendingID, *t.pending, true
}

func (t *modPowerTurnTracker) RestorePending(id string, a efficiency.RepairAttempt) {
	if t == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = newModPowerPendingID()
	}
	t.mu.Lock()
	cp := a
	t.pending = &cp
	t.pendingID = id
	t.snap.StrategyID = strings.TrimSpace(a.StrategyID)
	t.snap.LastFailureHash = strings.TrimSpace(a.FailureFingerprint)
	t.snap.Active = false
	t.snap.PendingApplication = true
	t.snap.LastDecision = "restored_after_restart"
	t.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	t.mu.Unlock()
}

func newModPowerPendingID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return modPowerHash(fmt.Sprintf("%d", time.Now().UnixNano()))
}

func (t *modPowerTurnTracker) Pending() (efficiency.RepairAttempt, bool) {
	if t == nil {
		return efficiency.RepairAttempt{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending == nil {
		return efficiency.RepairAttempt{}, false
	}
	return *t.pending, true
}

func (t *modPowerTurnTracker) Applied(decision string, clear bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.snap.PendingApplication = !clear && t.pending != nil
	t.snap.LastDecision = decision
	if clear {
		t.pending = nil
		t.pendingID = ""
		t.snap.PendingApplication = false
	}
	t.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	t.mu.Unlock()
}

func modPowerHash(s string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return hex.EncodeToString(sum[:12])
}

func (s *Server) observePowerCoreEvent(e event.Event) {
	if s.modPowerTurn == nil {
		return
	}
	if e.Kind == event.TurnStarted {
		exec := s.modExec.Snapshot()
		strategy := string(exec.Mode)
		if strategy == "" || strategy == string(efficiency.ExecutionDisabled) {
			strategy = "native:" + currentModelRef(s.ctl())
		}
		s.modPowerTurn.Begin(modPowerFirstNonEmpty(e.ModelRef, currentModelRef(s.ctl())), strategy)
		return
	}

	s.modPowerTurn.Observe(e)
	if e.Kind != event.TurnDone || e.Cancelled {
		return
	}
	attempt, ok := s.modPowerTurn.Finish(e)
	if !ok {
		return
	}
	s.modHub.Emit("power.turn.observed", s.modPowerTurn.Snapshot())
	s.modHub.Emit("power.route.pending", map[string]any{"strategyId": attempt.StrategyID, "failureFingerprint": attempt.FailureFingerprint != "", "finalization": attempt.Finalization})
	_ = attempt // the content-bearing attempt stays private in modPowerTurn.pending
	// Persist the sanitized pending route before any model switch/continuation.
	// This closes the TurnDone -> enqueue crash-loss window for real workspaces.
	if err := s.persistModAppState(); err != nil {
		s.modAuto.noteBlocked("cannot durably persist pending power route: "+err.Error(), true)
		s.modHub.Emit("power.route.persistence_failed", map[string]any{"reason": err.Error()})
		return
	}
	// v0.12 schedules the route at a real idle boundary. The scheduler pauses
	// the native durable inbox before TurnDone fan-out finishes, then performs
	// model switching and continuation admission outside this callback. If any
	// safety gate refuses automation, the pending route remains available for
	// explicit APK review/apply instead of being silently discarded.
	if err := s.scheduleModAutoContinuation(); err != nil {
		s.modHub.Emit("orchestrator.pending", map[string]any{"reason": err.Error()})
	}
}

func modPowerFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
