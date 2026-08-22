package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"reasonix/internal/billing"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/efficiency"
	"reasonix/internal/event"
)

const balanceModVersion = "balance-mod-v0.20"

// modEvent is a typed machine event for the Android client. Most Balance Mod
// telemetry is content-free; v0.7 live.* events may additionally carry bounded,
// credential-redacted project details when the project profile opts into them.
// The APK never needs to parse terminal text.
type modEvent struct {
	Type       string `json:"type"`
	AtUnixMS   int64  `json:"atUnixMs"`
	Sequence   uint64 `json:"sequence"`
	Data       any    `json:"data,omitempty"`
	ModVersion string `json:"modVersion"`
}

type modBroadcaster struct {
	mu      sync.Mutex
	subs    map[chan []byte]struct{}
	seq     uint64
	history [][]byte
}

func newModBroadcaster() *modBroadcaster {
	return &modBroadcaster{subs: make(map[chan []byte]struct{})}
}

func (b *modBroadcaster) encode(kind string, data any) []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	b.seq++
	seq := b.seq
	b.mu.Unlock()
	payload, err := json.Marshal(modEvent{
		Type:       kind,
		AtUnixMS:   time.Now().UnixMilli(),
		Sequence:   seq,
		Data:       data,
		ModVersion: balanceModVersion,
	})
	if err != nil {
		return nil
	}
	return payload
}

func (b *modBroadcaster) emit(kind string, data any, retain bool) {
	payload := b.encode(kind, data)
	if len(payload) == 0 {
		return
	}
	b.mu.Lock()
	if retain {
		b.history = append(b.history, append([]byte(nil), payload...))
		if len(b.history) > 512 {
			b.history = append([][]byte(nil), b.history[len(b.history)-512:]...)
		}
	}
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- payload:
		default:
		}
	}
}

func (b *modBroadcaster) Emit(kind string, data any) { b.emit(kind, data, true) }

// EmitTransient streams high-frequency deltas without letting them evict
// durable action/result events from the bounded APK history ring.
func (b *modBroadcaster) EmitTransient(kind string, data any) { b.emit(kind, data, false) }

func (b *modBroadcaster) History(limit int) [][]byte {
	if b == nil {
		return nil
	}
	if limit <= 0 || limit > 512 {
		limit = 100
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	start := len(b.history) - limit
	if start < 0 {
		start = 0
	}
	out := make([][]byte, 0, len(b.history)-start)
	for _, item := range b.history[start:] {
		out = append(out, append([]byte(nil), item...))
	}
	return out
}

func (b *modBroadcaster) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 32)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

type modTaskCostGateStatus struct {
	Applied       bool    `json:"applied"`
	PreCall       bool    `json:"preCall"`
	SingleAgent   bool    `json:"singleAgent"`
	Currency      string  `json:"currency,omitempty"`
	ProviderLimit float64 `json:"providerLimit,omitempty"`
	Reason        string  `json:"reason,omitempty"`
}

type modStatusPayload struct {
	ModVersion   string                         `json:"modVersion"`
	Running      bool                           `json:"running"`
	ModelRef     string                         `json:"modelRef,omitempty"`
	Budget       efficiency.BudgetSnapshot      `json:"budget"`
	TaskGate     modTaskCostGateStatus          `json:"taskCostGate"`
	Resources    efficiency.ResourceSnapshot    `json:"resources"`
	Quality      efficiency.QualitySnapshot     `json:"quality"`
	Router       efficiency.RouteDecision       `json:"router"`
	Cycle        efficiency.RepairCycleSnapshot `json:"cycle"`
	Execution    efficiency.ExecutionSnapshot   `json:"execution"`
	Power        efficiency.PowerSnapshot       `json:"power"`
	Orchestrator modAutoSnapshot                `json:"orchestrator"`
	Profile      efficiency.ProjectProfile      `json:"profile"`
	Environment  modEnvironmentSnapshot         `json:"environment"`
	Persistence  map[string]any                 `json:"persistence"`
	Features     map[string]string              `json:"features"`
}

type taskCostBudgetSetter interface {
	SetTaskCostBudget(cost float64) error
}

type strictPreCallBudgetSetter interface {
	SetStrictPreCallBudget(enabled bool) error
}

func (s *Server) modWorkspacePath() string {
	ctl := s.ctl()
	if root := strings.TrimSpace(ctl.WorkspaceRoot()); root != "" {
		return root
	}
	if dir := strings.TrimSpace(ctl.SessionDir()); dir != "" {
		return dir
	}
	return "."
}

func (s *Server) modTaskGateSnapshot() modTaskCostGateStatus {
	s.modMu.RLock()
	defer s.modMu.RUnlock()
	return s.modTaskGate
}

func (s *Server) setModTaskGate(v modTaskCostGateStatus) {
	s.modMu.Lock()
	s.modTaskGate = v
	s.modMu.Unlock()
}

func (s *Server) modStatus(w http.ResponseWriter, _ *http.Request) {
	ctl := s.ctl()
	writeJSON(w, modStatusPayload{
		ModVersion:   balanceModVersion,
		Running:      ctl.Running(),
		ModelRef:     ctl.ModelRef(),
		Budget:       s.modGov.Snapshot(),
		TaskGate:     s.modTaskGateSnapshot(),
		Resources:    efficiency.ReadResources(s.modWorkspacePath()),
		Quality:      s.modQuality.Snapshot(),
		Router:       s.modRouter.Snapshot(),
		Cycle:        s.modCycle.Snapshot(),
		Execution:    s.modExec.Snapshot(),
		Power:        s.modPower.Snapshot(),
		Orchestrator: s.modAuto.Snapshot(),
		Profile:      s.modProfile.Snapshot(),
		Environment:  s.modEnvironmentSnapshot(context.Background(), false),
		Persistence:  s.modPersistenceStatus(),
		Features: map[string]string{
			"budget":               "pre-call-cap+post-round-ledger-v0.16",
			"apkBridge":            "http+sse-v0.2",
			"resources":            "local-linux-v0.1",
			"qualityMonitor":       "host-evidence-v0.2",
			"antiLoop":             "reasonix-native+apk-telemetry",
			"completionGate":       "reasonix-final-readiness",
			"offlineApiKey":        "not-required",
			"powerRouter":          "objective-flash-flash-pro-flash-v0.3",
			"failureCache":         "verified-journal-crc-v0.3",
			"logReducer":           "root-cause-window-v0.3",
			"patchGovernor":        "git-numstat-v0.3",
			"repairCycle":          "router+verifier+cache+rollback-v0.4",
			"nativeRollback":       "reasonix-checkpoint-transaction-v0.4",
			"executionRouter":      "native-model-switch+budget-admission-v0.5",
			"providerAgnostic":     "apk-configured-model-refs-v0.5",
			"agentControl":         "workspace+tools+skills+instructions-v0.6",
			"toolPolicy":           "native-permission-gate+schema-trim-v0.6",
			"workspaceFiles":       "confined-read-preview-v0.6",
			"capabilityRegistry":   "dynamic-tool-packs+native-policy-v0.7",
			"environmentDiscovery": "reasonix-native-probes+apk-project-markers-v0.7",
			"projectProfile":       "chat-agent-modes+tool-packs-v0.7",
			"liveProtocol":         "typed-actions-diff-results-no-hidden-reasoning-v0.7",
			"apkControlPlane":      "bootstrap+atomic-apply+native-task-aliases-v0.8",
			"apkStatePersistence":  "workspace-scoped-atomic+budget-ledger-v0.8",
			"projectManager":       "global-registry+supervisor-handoff-v0.9",
			"taskManager":          "native-session-catalog-no-provider-v0.9",
			"taskQueue":            "native-sessioninbox+reviewed-recovery+per-turn-budget-v0.10",
			"unifiedPowerEngine":   "repair+router+budget+execution-single-path-v0.11",
			"autoContinuation":     "idle-boundary+durable-inbox+submit-gate-v0.12",
			"durablePendingRoute":  "sanitized-state+stable-idempotency-v0.13",
			"proDiagnosisGuard":    "native-read-only-policy-overlay-v0.13",
			"androidSupervisor":    "debian-process-wrapper-v0.8",
			"offlineStressGate":    "native-deny+crash-restart+queue+corrupt-state-v0.15",
			"hardPreCallBudget":    "provider-max-output-cap+retry-reserve-v0.16",
			"hardBudgetSideCalls":  "single-agent+no-title+no-semantic+delegation-block-v0.16",
		},
	})
}

func (s *Server) modBudgetGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"budget":       s.modGov.Snapshot(),
		"taskCostGate": s.modTaskGateSnapshot(),
	})
}

func (s *Server) modBudgetSet(w http.ResponseWriter, r *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		http.Error(w, "cannot change budget during automatic continuation", http.StatusConflict)
		return
	}
	if s.ctl().Running() {
		http.Error(w, "cannot change budget while a turn is running", http.StatusConflict)
		return
	}
	cfg := efficiency.DefaultBudgetConfig()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		http.Error(w, "invalid budget config: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := efficiency.ValidateConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	snap := s.modGov.Configure(cfg)
	gate := s.applyModTaskCostBudget()
	s.modHub.Emit("budget.configured", map[string]any{"budget": snap, "taskCostGate": gate})
	s.persistModAppStateBestEffort()
	writeJSON(w, map[string]any{"budget": snap, "taskCostGate": gate})
}

func (s *Server) modBudgetReset(w http.ResponseWriter, _ *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		http.Error(w, "cannot reset budget during automatic continuation", http.StatusConflict)
		return
	}
	if s.ctl().Running() {
		http.Error(w, "cannot reset budget while a turn is running", http.StatusConflict)
		return
	}
	snap := s.modGov.ResetSpend()
	s.modHub.Emit("budget.reset", snap)
	s.persistModAppStateBestEffort()
	writeJSON(w, snap)
}

func (s *Server) modResources(w http.ResponseWriter, _ *http.Request) {
	snap := efficiency.ReadResources(s.modWorkspacePath())
	writeJSON(w, snap)
}

func (s *Server) modQualityGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.modQuality.Snapshot())
}

func (s *Server) modRouterGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.modRouter.Snapshot())
}

func (s *Server) modRouterReset(w http.ResponseWriter, _ *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot reset router while a turn is running", http.StatusConflict)
		return
	}
	cycle := s.modCycle.Reset()
	snap := cycle.LastRoute
	s.modHub.Emit("router.reset", snap)
	s.modHub.Emit("repair.reset", cycle)
	writeJSON(w, snap)
}

func (s *Server) modCycleGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.modCycle.Snapshot())
}

func (s *Server) modCycleReset(w http.ResponseWriter, _ *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot reset repair cycle while a turn is running", http.StatusConflict)
		return
	}
	snap := s.modCycle.Reset()
	s.modHub.Emit("repair.reset", snap)
	writeJSON(w, snap)
}

func (s *Server) modExecutionGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.modExec.Snapshot())
}

func (s *Server) modExecutionConfigure(w http.ResponseWriter, r *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		http.Error(w, "cannot change execution routing during automatic continuation", http.StatusConflict)
		return
	}
	if s.ctl().Running() {
		http.Error(w, "cannot change execution routing while a turn is running", http.StatusConflict)
		return
	}
	var cfg efficiency.ExecutionConfig
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		http.Error(w, "invalid execution config: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := efficiency.ValidateExecutionConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cfg.Enabled {
		for _, ref := range []string{cfg.FlashPrimaryRef, cfg.FlashAlternativeRef, cfg.ProRef, cfg.FlashRepairRef} {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			if !s.modExecutionRefKnown(ref) {
				http.Error(w, "unknown model ref: "+strings.TrimSpace(ref), http.StatusBadRequest)
				return
			}
		}
	}
	snap, err := s.modExec.Configure(cfg, currentModelRef(s.ctl()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.setModExecutionPolicyMode(snap.Mode)
	if err := s.applyModToolDecisionsToCurrentController(); err != nil {
		http.Error(w, "apply execution tool guard: "+err.Error(), http.StatusConflict)
		return
	}
	s.modHub.Emit("execution.configured", snap)
	s.persistModAppStateBestEffort()
	writeJSON(w, snap)
}

func (s *Server) modExecutionReset(w http.ResponseWriter, _ *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		http.Error(w, "cannot reset execution routing during automatic continuation", http.StatusConflict)
		return
	}
	if s.ctl().Running() {
		http.Error(w, "cannot reset execution routing while a turn is running", http.StatusConflict)
		return
	}
	snap := s.modExec.Reset(currentModelRef(s.ctl()))
	s.setModExecutionPolicyMode(snap.Mode)
	_ = s.applyModToolDecisionsToCurrentController()
	s.modHub.Emit("execution.reset", snap)
	s.persistModAppStateBestEffort()
	writeJSON(w, snap)
}

func (s *Server) modExecutionRefKnown(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	if strings.EqualFold(ref, currentModelRef(s.ctl())) {
		return true
	}
	if cfg, err := config.Load(); err == nil {
		if _, ok := cfg.ResolveModel(ref); ok {
			return true
		}
	}
	for _, d := range s.ctl().ProviderCatalog() {
		if strings.EqualFold(strings.TrimSpace(d.Ref), ref) {
			return true
		}
	}
	return false
}

type modRecoveryStatus struct {
	Available       bool   `json:"available"`
	CheckpointCount int    `json:"checkpointCount"`
	LatestTurn      int    `json:"latestTurn,omitempty"`
	Files           int    `json:"files,omitempty"`
	Coverage        string `json:"coverage,omitempty"`
	CoverageGaps    int    `json:"coverageGaps,omitempty"`
}

func (s *Server) modRecoverySnapshot() modRecoveryStatus {
	cps := s.ctl().Checkpoints()
	out := modRecoveryStatus{CheckpointCount: len(cps)}
	if len(cps) == 0 {
		return out
	}
	latest := cps[0]
	for _, cp := range cps[1:] {
		if cp.Turn > latest.Turn {
			latest = cp
		}
	}
	out.Available = true
	out.LatestTurn = latest.Turn
	out.Files = len(latest.Paths)
	out.Coverage = string(latest.Coverage)
	out.CoverageGaps = len(latest.CoverageGaps)
	return out
}

func (s *Server) modRecoveryGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.modRecoverySnapshot())
}

// modRollbackLatestCode deliberately delegates to the native Reasonix rewind
// transaction instead of implementing another file backup mechanism. Rewind
// itself refuses active turns, conflicts and partial-coverage restores that
// require explicit confirmation.
func (s *Server) modRollbackLatestCode(_ string) error {
	ctl := s.ctl()
	if ctl.Running() {
		return fmt.Errorf("cannot rollback while a turn is running")
	}
	cps := ctl.Checkpoints()
	if len(cps) == 0 {
		return fmt.Errorf("no checkpoint available")
	}
	latest := cps[0].Turn
	for _, cp := range cps[1:] {
		if cp.Turn > latest {
			latest = cp.Turn
		}
	}
	return ctl.Rewind(latest, control.RewindCode)
}

func (s *Server) modRecoveryRollbackLast(w http.ResponseWriter, _ *http.Request) {
	before := s.modRecoverySnapshot()
	if err := s.modRollbackLatestCode("APK requested rollback"); err != nil {
		s.modHub.Emit("rollback.failed", map[string]any{"reason": err.Error()})
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	after := s.modRecoverySnapshot()
	s.modHub.Emit("rollback.completed", map[string]any{"turn": before.LatestTurn})
	writeJSON(w, map[string]any{"ok": true, "before": before, "after": after})
}

// observeRepairAttempt is the controller-side repair wiring point. v0.5 also
// applies the deterministic route through Reasonix's native model-switch path.
// The background wrapper preserves the v0.4 internal test/API seam; production
// callers should prefer observeRepairAttemptCtx so cancellation propagates.
func (s *Server) observeRepairAttempt(a efficiency.RepairAttempt) (efficiency.RepairCycleResult, error) {
	return s.observeRepairAttemptCtx(context.Background(), a)
}

func (s *Server) observeRepairAttemptCtx(ctx context.Context, a efficiency.RepairAttempt) (efficiency.RepairCycleResult, error) {
	res, err := s.applyUnifiedPowerAttemptCtx(ctx, a)
	return res.Repair, err
}

func (s *Server) modEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := s.modHub.Subscribe()
	defer unsubscribe()

	initial := s.modHub.encode("mod.connected", map[string]any{
		"budget":      s.modGov.Snapshot(),
		"quality":     s.modQuality.Snapshot(),
		"resources":   efficiency.ReadResources(s.modWorkspacePath()),
		"router":      s.modRouter.Snapshot(),
		"cycle":       s.modCycle.Snapshot(),
		"execution":   s.modExec.Snapshot(),
		"power":       s.modPower.Snapshot(),
		"powerTurn":   s.modPowerTurn.Snapshot(),
		"recovery":    s.modRecoverySnapshot(),
		"profile":     s.modProfile.Snapshot(),
		"environment": s.modEnvironmentSnapshot(context.Background(), false),
		"persistence": s.modPersistenceStatus(),
		"agent":       map[string]any{"workspace": s.ctl().WorkspaceRoot(), "toolOverrides": len(s.modToolDecisionsSnapshot()), "skills": len(s.modSkillViews())},
	})
	if len(initial) > 0 {
		fmt.Fprintf(w, "data: %s\n\n", initial)
	}
	flusher.Flush()

	keepalive := time.NewTicker(sseKeepaliveInterval)
	defer keepalive.Stop()
	resources := time.NewTicker(5 * time.Second)
	defer resources.Stop()

	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-resources.C:
			data := s.modHub.encode("resource.snapshot", efficiency.ReadResources(s.modWorkspacePath()))
			if len(data) > 0 {
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		case <-keepalive.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// observeCoreEvent is the single native-event tap. The quality/budget path below
// remains content-free; observeLiveCoreEvent separately maps only the explicitly
// allowed visible chat/action/result surface and never forwards hidden reasoning.
func (s *Server) observeCoreEvent(e event.Event) {
	s.observeLiveCoreEvent(e)
	s.observePowerCoreEvent(e)
	quality, signals := s.modQuality.Observe(e)
	for _, sig := range signals {
		s.modHub.Emit(sig.Type, sig.Data)
	}
	if len(signals) > 0 {
		s.modHub.Emit("quality.updated", quality)
	}

	if e.Kind != event.Usage || e.Usage == nil {
		return
	}
	before := s.modGov.Snapshot()
	after := s.modGov.ObserveQuote(e.CostQuote, e.ModelRef)
	s.modHub.Emit("budget.updated", after)
	if !before.Exhausted && after.Exhausted {
		s.modHub.Emit("budget.exhausted", after)
	}
	if before.SpentKZT < before.RegularLimitKZT && after.SpentKZT >= after.RegularLimitKZT && after.Enabled {
		s.modHub.Emit("budget.reserve_entered", after)
	}
	if !before.ProLimitReached && after.ProLimitReached {
		s.modHub.Emit("budget.pro_limit_reached", after)
	}
	// Persist accounted spend after every provider Usage event. This prevents a
	// backend/process restart from silently granting a fresh KZT budget.
	s.persistModAppStateBestEffort()
}

// applyModTaskCostBudget binds the global KZT hard-stop policy to the current
// controller's provider-currency task budget and strict pre-call guard. It uses
// only the remaining workspace allowance, so controller/model rebuilds cannot
// re-grant already-spent money. Missing currency/FX is fail-closed while
// hardStop is requested; ordinary Reasonix behavior is unchanged when it is off.
func (s *Server) applyModTaskCostBudget() modTaskCostGateStatus {
	snap := s.modGov.Snapshot()
	setter, ok := s.ctl().(taskCostBudgetSetter)
	if !ok {
		status := modTaskCostGateStatus{Reason: "controller does not expose task-cost budget setter"}
		s.setModTaskGate(status)
		return status
	}
	strictSetter, strictOK := s.ctl().(strictPreCallBudgetSetter)
	setStrict := func(enabled bool) error {
		if !strictOK {
			if enabled {
				return fmt.Errorf("controller does not expose strict pre-call budget setter")
			}
			return nil
		}
		return strictSetter.SetStrictPreCallBudget(enabled)
	}
	if !snap.Enabled || !snap.HardStop {
		_ = setStrict(false)
		if err := setter.SetTaskCostBudget(0); err != nil {
			status := modTaskCostGateStatus{Reason: err.Error()}
			s.setModTaskGate(status)
			return status
		}
		status := modTaskCostGateStatus{Applied: false, PreCall: false, SingleAgent: false, Reason: "hard budget disabled"}
		s.setModTaskGate(status)
		return status
	}

	// A requested hard stop must fail closed when provider pricing/FX cannot be
	// resolved. Keep strict mode on with a zero local cost allowance so even a
	// non-HTTP/native continuation cannot slip through while the APK sees the
	// explicit reason in taskCostGate.
	failClosed := func(currency, reason string) modTaskCostGateStatus {
		_ = setter.SetTaskCostBudget(0)
		preCall := true
		if err := setStrict(true); err != nil {
			preCall = false
			if reason != "" {
				reason += "; "
			}
			reason += err.Error()
		}
		status := modTaskCostGateStatus{Applied: false, PreCall: preCall, SingleAgent: preCall, Currency: currency, Reason: reason}
		s.setModTaskGate(status)
		return status
	}

	currency, reason := s.activeProviderBillingCurrency()
	if currency == "" {
		return failClosed("", reason)
	}
	providerLimit, ok := s.modGov.RemainingProviderBudget(currency)
	if !ok {
		return failClosed(currency, "missing KZT FX rate for "+currency)
	}
	if err := setter.SetTaskCostBudget(providerLimit); err != nil {
		_ = setStrict(false)
		status := modTaskCostGateStatus{Currency: currency, ProviderLimit: providerLimit, Reason: err.Error()}
		s.setModTaskGate(status)
		return status
	}
	if err := setStrict(true); err != nil {
		_ = setter.SetTaskCostBudget(0)
		status := modTaskCostGateStatus{Currency: currency, ProviderLimit: providerLimit, Reason: err.Error()}
		s.setModTaskGate(status)
		return status
	}
	status := modTaskCostGateStatus{Applied: true, PreCall: true, SingleAgent: true, Currency: currency, ProviderLimit: providerLimit}
	s.setModTaskGate(status)
	return status
}

func (s *Server) modHardBudgetAdmissionReady() (bool, string) {
	if s == nil || s.modGov == nil {
		return true, ""
	}
	snap := s.modGov.Snapshot()
	if !snap.Enabled || !snap.HardStop {
		return true, ""
	}
	if snap.Exhausted || snap.RemainingKZT <= 0 {
		return false, "workspace KZT budget is exhausted"
	}
	gate := s.modTaskGateSnapshot()
	if !gate.PreCall || !gate.Applied {
		reason := strings.TrimSpace(gate.Reason)
		if reason == "" {
			reason = "hard pre-call provider budget is unavailable"
		}
		return false, reason
	}
	if gate.ProviderLimit <= 0 {
		return false, "workspace provider budget has no remaining allowance"
	}
	return true, ""
}

func (s *Server) activeProviderBillingCurrency() (string, string) {
	cfg, err := config.Load()
	if err != nil {
		return "", "cannot load provider config"
	}
	ref := strings.TrimSpace(s.ctl().ModelRef())
	if ref == "" {
		ref = strings.TrimSpace(cfg.DefaultModel)
	}
	entry, ok := cfg.ResolveModel(ref)
	if !ok || entry == nil {
		return "", "active model pricing is unavailable"
	}
	if cur := billing.NormalizeCurrency(entry.BillingCurrency); cur != "" {
		return cur, ""
	}
	if entry.Price != nil {
		if cur := billing.NormalizeCurrency(entry.Price.Currency); cur != "" {
			return cur, ""
		}
	}
	return "", "active model has no billing currency"
}
