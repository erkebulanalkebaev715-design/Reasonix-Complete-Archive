package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/efficiency"
	"reasonix/internal/sessioninbox"
)

// modAutoConfig controls host-owned continuation between verified repair turns.
// It never grants model permissions and never bypasses the existing budget,
// execution-router, permission, verifier, or durable-inbox gates.
type modAutoConfig struct {
	Enabled          bool `json:"enabled"`
	MaxContinuations int  `json:"maxContinuations"`
	IdleWaitSeconds  int  `json:"idleWaitSeconds"`
}

func defaultModAutoConfig() modAutoConfig {
	return modAutoConfig{Enabled: true, MaxContinuations: 4, IdleWaitSeconds: 15}
}

func normalizeModAutoConfig(c modAutoConfig) modAutoConfig {
	if c.MaxContinuations == 0 {
		c.MaxContinuations = 4
	}
	if c.IdleWaitSeconds == 0 {
		c.IdleWaitSeconds = 15
	}
	return c
}

func validateModAutoConfig(c modAutoConfig) error {
	c = normalizeModAutoConfig(c)
	if c.MaxContinuations < 1 || c.MaxContinuations > 12 {
		return fmt.Errorf("maxContinuations must be between 1 and 12")
	}
	if c.IdleWaitSeconds < 1 || c.IdleWaitSeconds > 120 {
		return fmt.Errorf("idleWaitSeconds must be between 1 and 120")
	}
	return nil
}

type modAutoSnapshot struct {
	Config            modAutoConfig `json:"config"`
	State             string        `json:"state"`
	Active            bool          `json:"active"`
	SubmitGate        bool          `json:"submitGate"`
	PendingRoute      bool          `json:"pendingRoute"`
	Continuations     int           `json:"continuations"`
	LastAction        string        `json:"lastAction,omitempty"`
	LastQueueItemID   string        `json:"lastQueueItemId,omitempty"`
	LastReason        string        `json:"lastReason,omitempty"`
	StartedAtUnixMS   int64         `json:"startedAtUnixMs,omitempty"`
	UpdatedAtUnixMS   int64         `json:"updatedAtUnixMs"`
	AutomaticDispatch bool          `json:"automaticDispatch"`
}

// modAutoOrchestrator owns only the continuation lease/state. Actual model
// switching stays in PowerEngine/ExecutionRouter and actual work admission stays
// in Reasonix's native durable session inbox.
type modAutoOrchestrator struct {
	mu     sync.Mutex
	cfg    modAutoConfig
	snap   modAutoSnapshot
	seq    uint64
	cancel context.CancelFunc
}

func newModAutoOrchestrator() *modAutoOrchestrator {
	c := defaultModAutoConfig()
	return &modAutoOrchestrator{cfg: c, snap: modAutoSnapshot{Config: c, State: "idle", AutomaticDispatch: true, UpdatedAtUnixMS: time.Now().UnixMilli()}}
}

func (o *modAutoOrchestrator) Snapshot() modAutoSnapshot {
	if o == nil {
		return modAutoSnapshot{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snap
}

func (o *modAutoOrchestrator) Configure(c modAutoConfig) (modAutoSnapshot, error) {
	if o == nil {
		return modAutoSnapshot{}, fmt.Errorf("orchestrator unavailable")
	}
	c = normalizeModAutoConfig(c)
	if err := validateModAutoConfig(c); err != nil {
		return modAutoSnapshot{}, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.snap.Active {
		return o.snap, fmt.Errorf("cannot reconfigure orchestrator during an active continuation transition")
	}
	o.cfg = c
	o.snap.Config = c
	o.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	if !c.Enabled {
		o.snap.State = "disabled"
		o.snap.SubmitGate = false
	} else if o.snap.State == "disabled" {
		o.snap.State = "idle"
	}
	return o.snap, nil
}

func (o *modAutoOrchestrator) tryBegin() (context.Context, uint64, bool, string) {
	if o == nil {
		return nil, 0, false, "orchestrator unavailable"
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.cfg.Enabled {
		o.snap.State = "disabled"
		o.snap.PendingRoute = true
		o.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
		return nil, 0, false, "automatic continuation is disabled"
	}
	if o.snap.Active {
		return nil, 0, false, "continuation transition already active"
	}
	if o.snap.State == "idle" || o.snap.State == "terminal" {
		o.snap.Continuations = 0
	}
	if o.snap.Continuations >= o.cfg.MaxContinuations {
		o.snap.State = "limit_reached"
		o.snap.PendingRoute = true
		o.snap.LastReason = "automatic continuation limit reached"
		o.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
		return nil, 0, false, o.snap.LastReason
	}
	o.seq++
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = cancel
	o.snap.Active = true
	o.snap.SubmitGate = true
	o.snap.PendingRoute = true
	o.snap.State = "waiting_idle"
	o.snap.LastReason = ""
	o.snap.StartedAtUnixMS = time.Now().UnixMilli()
	o.snap.UpdatedAtUnixMS = o.snap.StartedAtUnixMS
	return ctx, o.seq, true, ""
}

func (o *modAutoOrchestrator) isCurrent(seq uint64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snap.Active && o.seq == seq
}

func (o *modAutoOrchestrator) stage(seq uint64, state, action, reason string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.seq != seq {
		return
	}
	o.snap.State = state
	if action != "" {
		o.snap.LastAction = action
	}
	if reason != "" {
		o.snap.LastReason = reason
	}
	o.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
}

func (o *modAutoOrchestrator) finish(seq uint64, state, action, reason, itemID string, consumed bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.seq != seq {
		return
	}
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
	o.snap.Active = false
	o.snap.SubmitGate = false
	o.snap.PendingRoute = !consumed
	o.snap.State = state
	if action != "" {
		o.snap.LastAction = action
	}
	if reason != "" {
		o.snap.LastReason = reason
	}
	if itemID != "" {
		o.snap.LastQueueItemID = itemID
	}
	if state == "queued" || state == "running" {
		o.snap.Continuations++
	}
	o.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
}

func (o *modAutoOrchestrator) Reset() modAutoSnapshot {
	if o == nil {
		return modAutoSnapshot{}
	}
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
	o.seq++
	o.snap = modAutoSnapshot{Config: o.cfg, State: "idle", AutomaticDispatch: true, UpdatedAtUnixMS: time.Now().UnixMilli()}
	if !o.cfg.Enabled {
		o.snap.State = "disabled"
	}
	out := o.snap
	o.mu.Unlock()
	return out
}

func (o *modAutoOrchestrator) Stop(reason string) modAutoSnapshot {
	if o == nil {
		return modAutoSnapshot{}
	}
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
	o.seq++
	o.snap.Active = false
	o.snap.SubmitGate = false
	o.snap.State = "stopped"
	o.snap.LastReason = strings.TrimSpace(reason)
	o.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	out := o.snap
	o.mu.Unlock()
	return out
}

func (o *modAutoOrchestrator) noteBlocked(reason string, pending bool) modAutoSnapshot {
	if o == nil {
		return modAutoSnapshot{}
	}
	o.mu.Lock()
	o.snap.Active = false
	o.snap.SubmitGate = false
	o.snap.PendingRoute = pending
	o.snap.State = "blocked"
	o.snap.LastReason = strings.TrimSpace(reason)
	o.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	out := o.snap
	o.mu.Unlock()
	return out
}

func (o *modAutoOrchestrator) BlocksSubmit() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snap.SubmitGate
}

func modAutoContinuationText(action efficiency.RouteAction) (display, prompt string, ok bool) {
	switch action {
	case efficiency.RouteRetryFlash:
		return "Balance Auto · alternative Flash repair",
			"Continue the current task using a materially different approach from the previous failed attempt. Inspect the current workspace state before editing. Do not repeat the same failed strategy. Keep the patch focused and run the relevant build/tests/verifier before claiming completion.", true
	case efficiency.RouteDiagnosePro:
		return "Balance Auto · Pro diagnosis",
			"Diagnose the unresolved technical failure in the current task. Focus on the root cause and produce a concise concrete repair direction for the next executor. Avoid unrelated changes and do not claim completion without host verification.", true
	case efficiency.RouteExecuteFlash:
		return "Balance Auto · Flash repair after diagnosis",
			"Continue the current task as the repair executor. Use the latest diagnosis plus the current workspace state, make the smallest justified fix, and run the relevant build/tests/verifier before claiming completion.", true
	case efficiency.RouteContinueFlash:
		return "Balance Auto · continue Flash",
			"Continue the current task from the current workspace state. Make objective progress and run the relevant build/tests/verifier before claiming completion.", true
	default:
		return "", "", false
	}
}

func (s *Server) modAutoGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.modAuto.Snapshot())
}

func (s *Server) modAutoConfigure(w http.ResponseWriter, r *http.Request) {
	if s.ctl().Running() || s.modAuto.BlocksSubmit() {
		http.Error(w, "cannot change orchestrator config while work is active", http.StatusConflict)
		return
	}
	cfg := defaultModAutoConfig()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		http.Error(w, "invalid orchestrator config: "+err.Error(), http.StatusBadRequest)
		return
	}
	snap, err := s.modAuto.Configure(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.modHub.Emit("orchestrator.configured", snap)
	s.persistModAppStateBestEffort()
	writeJSON(w, snap)
}

func (s *Server) modAutoStop(w http.ResponseWriter, _ *http.Request) {
	before := s.modAuto.Snapshot()
	if before.Active && before.State == "applying_route" {
		http.Error(w, "route application is atomic; retry stop after the model-switch boundary finishes", http.StatusConflict)
		return
	}
	wasActive := before.Active
	snap := s.modAuto.Stop("stopped by APK")
	if wasActive {
		// An active orchestrator owns the temporary queue pause. Explicit stop
		// releases it; a queue that was already user-paused can never enter the
		// active state in the first place.
		_ = s.ctl().SetInboxPaused(false)
	}
	s.modHub.Emit("orchestrator.stopped", snap)
	writeJSON(w, snap)
}

func (s *Server) modAutoResume(w http.ResponseWriter, _ *http.Request) {
	if _, ok := s.modPowerTurn.Pending(); !ok {
		http.Error(w, "no pending power route", http.StatusConflict)
		return
	}
	if !s.modAuto.Snapshot().Config.Enabled {
		cfg := s.modAuto.Snapshot().Config
		cfg.Enabled = true
		if _, err := s.modAuto.Configure(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	if err := s.scheduleModAutoContinuation(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, s.modAuto.Snapshot())
}

func (s *Server) scheduleModAutoContinuation() error {
	if s.modAuto == nil {
		return fmt.Errorf("orchestrator unavailable")
	}
	if _, ok := s.modPowerTurn.Pending(); !ok {
		return fmt.Errorf("no pending power route")
	}
	if !s.modExec.Snapshot().Enabled {
		s.modAuto.noteBlocked("execution routing is disabled", true)
		return fmt.Errorf("execution routing is disabled")
	}
	api := s.ctl()
	modEnsureInboxSession(api)
	if strings.TrimSpace(api.SessionPath()) == "" {
		return fmt.Errorf("automatic continuation requires a persisted session")
	}
	queue := api.InboxSnapshot()
	if queue.Paused {
		return fmt.Errorf("task queue is paused; automatic continuation will not override a user pause")
	}
	ctx, seq, ok, reason := s.modAuto.tryBegin()
	if !ok {
		return fmt.Errorf("%s", reason)
	}
	// Pause synchronously while TurnDone fan-out is still active. This prevents
	// the native inbox dispatcher from starting an older queued user task on the
	// model tier selected for the repair continuation.
	if err := api.SetInboxPaused(true); err != nil {
		s.modAuto.finish(seq, "blocked", "", "cannot pause durable queue: "+err.Error(), "", false)
		return err
	}
	sessionPath := api.SessionPath()
	s.modHub.Emit("orchestrator.scheduled", map[string]any{"state": "waiting_idle"})
	go s.runModAutoContinuation(ctx, seq, sessionPath)
	return nil
}

func (s *Server) runModAutoContinuation(ctx context.Context, seq uint64, sessionPath string) {
	if err := s.waitModAutoIdle(ctx, seq); err != nil {
		s.finishModAutoBlocked(seq, "idle boundary unavailable: "+err.Error())
		return
	}
	if !s.modAuto.isCurrent(seq) {
		return
	}
	if current := strings.TrimSpace(s.ctl().SessionPath()); current != strings.TrimSpace(sessionPath) {
		reason := "session changed before continuation could be applied"
		s.modAuto.finish(seq, "blocked", "", reason, "", false)
		s.modHub.Emit("orchestrator.blocked", map[string]any{"reason": reason})
		return
	}

	pendingID, attempt, ok := s.modPowerTurn.PendingRecord()
	if !ok {
		s.restoreModAutoQueue()
		s.modAuto.finish(seq, "idle", "", "pending route disappeared", "", true)
		return
	}

	// Prepare the durable turn budget before changing models. If KZT cannot be
	// enforced for the configured execution models, fail closed and leave the
	// route pending for explicit review instead of switching tiers first.
	extra := map[string]string{}
	budgetSnap := s.modGov.Snapshot()
	if budgetSnap.Enabled && budgetSnap.HardStop {
		if budgetSnap.RemainingKZT <= 0 {
			s.finishModAutoBlocked(seq, "workspace KZT budget is exhausted")
			return
		}
		var err error
		extra, _, err = s.modQueueBudgetExtras(modQueueBudgetRequest{BudgetKZT: budgetSnap.RemainingKZT})
		if err != nil {
			s.finishModAutoBlocked(seq, err.Error())
			return
		}
	}

	s.modAuto.stage(seq, "applying_route", "", "")
	res, err := s.applyUnifiedPowerAttemptCtx(ctx, attempt)
	if err != nil {
		s.modPowerTurn.Applied("blocked", false)
		s.finishModAutoBlocked(seq, err.Error())
		return
	}
	if !s.modAuto.isCurrent(seq) {
		return
	}
	action := res.Snapshot.NextAction
	if res.Snapshot.Terminal || res.Snapshot.Blocked {
		s.modPowerTurn.Applied(string(action), true)
		s.persistModAppStateBestEffort()
		s.restoreModAutoQueue()
		state := "terminal"
		if res.Snapshot.Blocked {
			state = "blocked"
		}
		s.modAuto.finish(seq, state, string(action), res.Snapshot.Reason, "", true)
		s.modHub.Emit("orchestrator.finished", s.modAuto.Snapshot())
		return
	}

	display, prompt, ok := modAutoContinuationText(action)
	if !ok {
		s.finishModAutoBlocked(seq, "route has no automatic continuation mapping: "+string(action))
		return
	}
	api := s.ctl()
	if strings.TrimSpace(api.SessionPath()) != strings.TrimSpace(sessionPath) {
		reason := "session changed after route application"
		s.modAuto.finish(seq, "blocked", string(action), reason, "", false)
		s.modHub.Emit("orchestrator.blocked", map[string]any{"reason": reason})
		return
	}

	idem := fmt.Sprintf("balance-auto-v13-%s-%s", pendingID, action)
	rec, err := api.EnqueueInbox(control.InboxRequest{
		Intent:      sessioninbox.IntentFollowup,
		Display:     display,
		Raw:         prompt,
		Submit:      prompt,
		Source:      "balance-auto",
		Idempotency: idem,
		Extra:       extra,
	})
	if err != nil {
		s.finishModAutoBlocked(seq, "cannot persist automatic continuation: "+err.Error())
		return
	}
	// A completed idempotency receipt means this exact continuation already ran
	// before a crash. Clear the restored pending route instead of duplicating the
	// model turn. A live queued replay keeps the original item and may be moved
	// to the front without creating another durable record.
	if rec.Idempotent && rec.Position == 0 {
		s.modPowerTurn.Applied(string(action), true)
		s.persistModAppStateBestEffort()
		s.restoreModAutoQueue()
		s.modAuto.finish(seq, "recovered_completed", string(action), "durable continuation already completed", rec.ItemID, true)
		s.modHub.Emit("orchestrator.recovered", map[string]any{"action": action, "itemId": rec.ItemID, "state": "completed"})
		return
	}
	if rec.Position > 1 {
		if err := api.MoveInboxItem(rec.ItemID, 0); err != nil {
			if !rec.Idempotent {
				_ = api.DeleteInboxItem(rec.ItemID)
			}
			s.finishModAutoBlocked(seq, "cannot prioritize automatic continuation: "+err.Error())
			return
		}
	}

	// The continuation is now durably represented by the native inbox. Clearing
	// our pending route after that boundary is safe: on a crash before this save,
	// the stable idempotency key above converts recovery into a replay hit.
	s.modPowerTurn.Applied(string(action), true)
	s.persistModAppStateBestEffort()
	s.modAuto.stage(seq, "queued", string(action), "")
	// Resuming the queue invokes Reasonix's native FIFO dispatcher synchronously.
	// Because the auto item was moved to index 0, it owns the next admission.
	if err := api.SetInboxPaused(false); err != nil {
		s.modAuto.finish(seq, "blocked", string(action), "cannot resume durable queue: "+err.Error(), rec.ItemID, true)
		return
	}
	state := "queued"
	if api.Running() {
		state = "running"
	}
	s.modAuto.finish(seq, state, string(action), "", rec.ItemID, true)
	s.modHub.Emit("orchestrator.continuation", map[string]any{"action": action, "itemId": rec.ItemID, "state": state})
}

func (s *Server) waitModAutoIdle(ctx context.Context, seq uint64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := s.modAuto.Snapshot().Config
	deadline := time.NewTimer(time.Duration(cfg.IdleWaitSeconds) * time.Second)
	defer deadline.Stop()

	if waiter, ok := any(s.ctl()).(interface {
		TurnFinishingDone() (<-chan struct{}, bool)
	}); ok {
		if done, active := waiter.TurnFinishingDone(); active {
			select {
			case <-done:
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				return fmt.Errorf("timed out waiting for TurnDone boundary")
			}
		}
	}

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !s.modAuto.isCurrent(seq) {
			return context.Canceled
		}
		if !controllerHasActiveRuntimeWork(s.ctl()) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for controller idle state")
		case <-ticker.C:
		}
	}
}

func (s *Server) restoreModAutoQueue() {
	api := s.ctl()
	if snap := api.InboxSnapshot(); snap.Paused {
		_ = api.SetInboxPaused(false)
	}
}

func (s *Server) finishModAutoBlocked(seq uint64, reason string) {
	if s.modAuto == nil || !s.modAuto.isCurrent(seq) {
		return
	}
	s.restoreModAutoQueue()
	s.modAuto.finish(seq, "blocked", "", reason, "", false)
	s.modHub.Emit("orchestrator.blocked", map[string]any{"reason": reason})
}
