package serve

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"reasonix/internal/billing"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/sessioninbox"
)

const (
	modTaskBudgetCostKey         = "reasonix.task_budget.cost"
	modTaskBudgetTokensKey       = "reasonix.task_budget.tokens"
	modTaskBudgetWallMSKey       = "reasonix.task_budget.wall_ms"
	modTaskBudgetKZTKey          = "balance.task_budget.kzt"
	modTaskBudgetRequestedKZTKey = "balance.task_budget.requested_kzt"
	modTaskBudgetCurrencyKey     = "balance.task_budget.currency"
)

type modQueueBudgetRequest struct {
	BudgetKZT   float64 `json:"budgetKzt,omitempty"`
	TokenLimit  int     `json:"tokenLimit,omitempty"`
	WallSeconds int     `json:"wallSeconds,omitempty"`
}

type modQueueBudgetView struct {
	RequestedKZT      float64 `json:"requestedKzt,omitempty"`
	EffectiveKZT      float64 `json:"effectiveKzt,omitempty"`
	ProviderCurrency  string  `json:"providerCurrency,omitempty"`
	ProviderCostLimit float64 `json:"providerCostLimit,omitempty"`
	TokenLimit        int     `json:"tokenLimit,omitempty"`
	WallSeconds       int     `json:"wallSeconds,omitempty"`
	Enforcement       string  `json:"enforcement,omitempty"`
}

type modQueueItemView struct {
	ID          string             `json:"id"`
	Intent      string             `json:"intent"`
	State       string             `json:"state"`
	Position    int                `json:"position"`
	Preview     string             `json:"preview,omitempty"`
	Source      string             `json:"source,omitempty"`
	CreatedAtMS int64              `json:"createdAtUnixMs,omitempty"`
	UpdatedAtMS int64              `json:"updatedAtUnixMs,omitempty"`
	BlockReason string             `json:"blockReason,omitempty"`
	TaskBudget  modQueueBudgetView `json:"taskBudget,omitempty"`
}

func validateModQueueBudget(req modQueueBudgetRequest) error {
	if math.IsNaN(req.BudgetKZT) || math.IsInf(req.BudgetKZT, 0) || req.BudgetKZT < 0 || req.BudgetKZT > 1_000_000_000 {
		return fmt.Errorf("budgetKzt must be a finite value between 0 and 1000000000")
	}
	if req.TokenLimit < 0 || req.TokenLimit > 1_000_000_000 {
		return fmt.Errorf("tokenLimit must be between 0 and 1000000000")
	}
	if req.WallSeconds < 0 || req.WallSeconds > 7*24*60*60 {
		return fmt.Errorf("wallSeconds must be between 0 and 604800")
	}
	return nil
}

func (s *Server) modQueueBudgetExtras(req modQueueBudgetRequest) (map[string]string, modQueueBudgetView, error) {
	if err := validateModQueueBudget(req); err != nil {
		return nil, modQueueBudgetView{}, err
	}
	view := modQueueBudgetView{RequestedKZT: req.BudgetKZT, EffectiveKZT: req.BudgetKZT, TokenLimit: req.TokenLimit, WallSeconds: req.WallSeconds}
	extra := map[string]string{}
	if req.TokenLimit > 0 {
		extra[modTaskBudgetTokensKey] = strconv.Itoa(req.TokenLimit)
	}
	if req.WallSeconds > 0 {
		extra[modTaskBudgetWallMSKey] = strconv.FormatInt(int64(req.WallSeconds)*1000, 10)
	}
	if req.BudgetKZT <= 0 {
		if req.TokenLimit > 0 || req.WallSeconds > 0 {
			view.Enforcement = "native-agent-task-budget"
		}
		return extra, view, nil
	}

	workspaceBudget := s.modGov.Snapshot()
	if workspaceBudget.Enabled && workspaceBudget.HardStop {
		if workspaceBudget.RemainingKZT <= 0 {
			return nil, view, fmt.Errorf("workspace KZT budget is exhausted")
		}
		if view.EffectiveKZT > workspaceBudget.RemainingKZT {
			view.EffectiveKZT = workspaceBudget.RemainingKZT
		}
	}
	currency, reason := s.modQueueBillingCurrency()
	if currency == "" {
		return nil, view, fmt.Errorf("cannot enforce task KZT budget: %s", reason)
	}
	currency = billing.NormalizeCurrency(currency)
	rate := workspaceBudget.FXKZTPerUnit[currency]
	if currency == "KZT" && rate == 0 {
		rate = 1
	}
	if rate <= 0 {
		return nil, view, fmt.Errorf("cannot enforce task KZT budget: missing KZT FX rate for %s", currency)
	}
	providerLimit := view.EffectiveKZT / rate
	if providerLimit <= 0 || math.IsNaN(providerLimit) || math.IsInf(providerLimit, 0) {
		return nil, view, fmt.Errorf("cannot enforce task KZT budget for active provider")
	}
	extra[modTaskBudgetCostKey] = strconv.FormatFloat(providerLimit, 'g', -1, 64)
	extra[modTaskBudgetRequestedKZTKey] = strconv.FormatFloat(view.RequestedKZT, 'g', -1, 64)
	extra[modTaskBudgetKZTKey] = strconv.FormatFloat(view.EffectiveKZT, 'g', -1, 64)
	extra[modTaskBudgetCurrencyKey] = currency
	view.ProviderCurrency = currency
	view.ProviderCostLimit = providerLimit
	view.Enforcement = "native-agent-task-budget+workspace-kzt-budget"
	return extra, view, nil
}

func uniqueModBillingCurrency(currencies []string) (string, bool) {
	seen := map[string]struct{}{}
	for _, raw := range currencies {
		cur := billing.NormalizeCurrency(raw)
		if cur == "" {
			return "", false
		}
		seen[cur] = struct{}{}
	}
	if len(seen) != 1 {
		return "", false
	}
	for cur := range seen {
		return cur, true
	}
	return "", false
}

// modQueueBillingCurrency returns one currency only when every model that the
// configured execution router may select prices in that same currency. Native
// Agent TaskBudget.Cost accumulates a scalar and therefore must never be used
// across mixed billing currencies. Mixed/unpriced routing fails closed.
func (s *Server) modQueueBillingCurrency() (string, string) {
	cfg, err := config.Load()
	if err != nil {
		return "", "cannot load provider config"
	}
	refs := []string{}
	execSnap := s.modExec.Snapshot()
	if execSnap.Enabled {
		for _, ref := range []string{execSnap.Config.FlashPrimaryRef, execSnap.Config.FlashAlternativeRef, execSnap.Config.ProRef, execSnap.Config.FlashRepairRef} {
			ref = strings.TrimSpace(ref)
			if ref != "" {
				refs = append(refs, ref)
			}
		}
	}
	if len(refs) == 0 {
		ref := strings.TrimSpace(s.ctl().ModelRef())
		if ref == "" {
			ref = strings.TrimSpace(cfg.DefaultModel)
		}
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return "", "no active model is configured"
	}
	currencies := make([]string, 0, len(refs))
	seenRefs := map[string]bool{}
	for _, ref := range refs {
		if seenRefs[ref] {
			continue
		}
		seenRefs[ref] = true
		entry, ok := cfg.ResolveModel(ref)
		if !ok || entry == nil {
			return "", "execution model pricing is unavailable: " + ref
		}
		cur := billing.NormalizeCurrency(entry.ProviderBillingCurrency())
		if cur == "" {
			return "", "execution model has no billing currency: " + ref
		}
		currencies = append(currencies, cur)
	}
	cur, ok := uniqueModBillingCurrency(currencies)
	if !ok {
		return "", "execution models use mixed billing currencies"
	}
	return cur, ""
}

func modQueueBudgetFromExtra(extra map[string]string) modQueueBudgetView {
	var out modQueueBudgetView
	if len(extra) == 0 {
		return out
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(extra[modTaskBudgetRequestedKZTKey]), 64); err == nil && v > 0 {
		out.RequestedKZT = v
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(extra[modTaskBudgetKZTKey]), 64); err == nil && v > 0 {
		out.EffectiveKZT = v
		if out.RequestedKZT == 0 {
			out.RequestedKZT = v
		}
	}
	out.ProviderCurrency = strings.TrimSpace(extra[modTaskBudgetCurrencyKey])
	if v, err := strconv.ParseFloat(strings.TrimSpace(extra[modTaskBudgetCostKey]), 64); err == nil && v > 0 {
		out.ProviderCostLimit = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(extra[modTaskBudgetTokensKey])); err == nil && v > 0 {
		out.TokenLimit = v
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(extra[modTaskBudgetWallMSKey]), 10, 64); err == nil && v > 0 {
		out.WallSeconds = int(v / 1000)
	}
	if out.ProviderCostLimit > 0 || out.TokenLimit > 0 || out.WallSeconds > 0 {
		out.Enforcement = "native-agent-task-budget"
	}
	if out.EffectiveKZT > 0 {
		out.Enforcement = "native-agent-task-budget+workspace-kzt-budget"
	}
	return out
}

func modQueueStateCounts(items []sessioninbox.InboxItemMeta) map[string]int {
	out := map[string]int{}
	for _, item := range items {
		out[string(item.State)]++
	}
	return out
}

func modQueueRecoverySummary(snap sessioninbox.InboxSnapshot) map[string]any {
	counts := modQueueStateCounts(snap.Items)
	return map[string]any{
		"recovered":       snap.Recovered,
		"recoveredCount":  snap.RecoveredN,
		"paused":          snap.Paused,
		"uncertain":       counts[string(sessioninbox.StateUncertain)],
		"blocked":         counts[string(sessioninbox.StateBlocked)],
		"requiresReview":  snap.Paused && (snap.Recovered || counts[string(sessioninbox.StateUncertain)] > 0 || counts[string(sessioninbox.StateBlocked)] > 0),
		"automaticResume": false,
	}
}

func (s *Server) modQueueView() map[string]any {
	snap := s.ctl().InboxSnapshot()
	items := make([]modQueueItemView, 0, len(snap.Items))
	for i, meta := range snap.Items {
		view := modQueueItemView{
			ID: meta.ID, Intent: string(meta.Intent), State: string(meta.State), Position: i,
			Preview: modBoundedRedacted(meta.Preview, 512), Source: modBoundedRedacted(meta.Source, 64),
			CreatedAtMS: meta.CreatedAt.UnixMilli(), UpdatedAtMS: meta.UpdatedAt.UnixMilli(),
			BlockReason: modBoundedRedacted(meta.BlockReason, 512),
		}
		if _, env, err := s.ctl().ReadInboxItem(meta.ID); err == nil {
			view.TaskBudget = modQueueBudgetFromExtra(env.Extra)
		}
		items = append(items, view)
	}
	return map[string]any{
		"schemaVersion":  snap.SchemaVersion,
		"revision":       snap.Revision,
		"paused":         snap.Paused,
		"recovered":      snap.Recovered,
		"recoveredCount": snap.RecoveredN,
		"readonly":       snap.Readonly,
		"items":          items,
		"count":          len(items),
		"stateCounts":    modQueueStateCounts(snap.Items),
		"capacity":       snap.Capacity,
		"recovery":       modQueueRecoverySummary(snap),
		"native": map[string]any{
			"backend":                 "reasonix-sessioninbox",
			"durable":                 true,
			"fifo":                    true,
			"providerCallsForListing": false,
			"storage":                 "per-session transactional inbox",
		},
	}
}

func (s *Server) modQueueGet(w http.ResponseWriter, _ *http.Request) { writeJSON(w, s.modQueueView()) }

func modEnsureInboxSession(api control.SessionAPI) {
	if ensurer, ok := any(api).(interface{ EnsureSessionPath() }); ok {
		ensurer.EnsureSessionPath()
	}
}

func (s *Server) modQueueEnqueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Input          string                `json:"input"`
		Intent         string                `json:"intent,omitempty"`
		IdempotencyKey string                `json:"idempotencyKey,omitempty"`
		FreezeRefs     []string              `json:"freezeRefs,omitempty"`
		TaskBudget     modQueueBudgetRequest `json:"taskBudget,omitempty"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.Input) == "" {
		http.Error(w, "invalid queue item", http.StatusBadRequest)
		return
	}
	extra, budgetView, err := s.modQueueBudgetExtras(body.TaskBudget)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	intent := sessioninbox.IntentFollowup
	if strings.EqualFold(strings.TrimSpace(body.Intent), string(sessioninbox.IntentSteer)) {
		intent = sessioninbox.IntentSteer
	} else if v := strings.TrimSpace(body.Intent); v != "" && !strings.EqualFold(v, string(sessioninbox.IntentFollowup)) {
		http.Error(w, "intent must be followup or steer", http.StatusBadRequest)
		return
	}
	api := s.ctl()
	modEnsureInboxSession(api)
	req := control.InboxRequest{
		Intent: intent, Display: body.Input, Raw: body.Input, Submit: body.Input,
		Source: "apk", Idempotency: strings.TrimSpace(body.IdempotencyKey),
		FreezeRefs: append([]string(nil), body.FreezeRefs...), Extra: extra,
	}
	var rec sessioninbox.InboxReceipt
	if intent == sessioninbox.IntentSteer {
		rec, err = api.TryEnqueueAndSteer(req)
	} else {
		rec, err = api.TryEnqueueFollowup(req)
	}
	if err != nil {
		writeInboxError(w, err)
		return
	}
	s.modHub.Emit("queue.item.admitted", map[string]any{"id": rec.ItemID, "disposition": rec.Disposition, "taskBudget": budgetView})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"receipt": rec, "taskBudget": budgetView})
}

func (s *Server) modQueueUpdate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Input string `json:"input"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if id == "" || dec.Decode(&body) != nil || strings.TrimSpace(body.Input) == "" {
		http.Error(w, "id and input are required", http.StatusBadRequest)
		return
	}
	meta, err := s.ctl().UpdateInboxItem(id, body.Input, body.Input, body.Input)
	if err != nil {
		writeInboxError(w, err)
		return
	}
	s.modHub.Emit("queue.item.updated", map[string]any{"id": id, "state": meta.State})
	writeJSON(w, meta)
}

func (s *Server) modQueueDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := s.ctl().DeleteInboxItem(id); err != nil {
		writeInboxError(w, err)
		return
	}
	s.modHub.Emit("queue.item.deleted", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) modQueueMove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`
		ToIndex int    `json:"toIndex"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.ID) == "" || body.ToIndex < 0 {
		http.Error(w, "valid id and toIndex are required", http.StatusBadRequest)
		return
	}
	if err := s.ctl().MoveInboxItem(strings.TrimSpace(body.ID), body.ToIndex); err != nil {
		writeInboxError(w, err)
		return
	}
	s.modHub.Emit("queue.reordered", map[string]any{"id": strings.TrimSpace(body.ID), "toIndex": body.ToIndex})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) modQueuePause(w http.ResponseWriter, _ *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		auto := s.modAuto.Snapshot()
		if auto.State == "applying_route" {
			http.Error(w, "cannot pause queue during atomic route application", http.StatusConflict)
			return
		}
		s.modAuto.Stop("queue paused by APK")
	}
	api := s.ctl()
	modEnsureInboxSession(api)
	if err := api.SetInboxPaused(true); err != nil {
		writeInboxError(w, err)
		return
	}
	s.modHub.Emit("queue.paused", map[string]any{"source": "apk"})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) modQueueResume(w http.ResponseWriter, _ *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		http.Error(w, "automatic continuation owns the temporary queue pause; stop the orchestrator before manual resume", http.StatusConflict)
		return
	}
	api := s.ctl()
	modEnsureInboxSession(api)
	if err := api.SetInboxPaused(false); err != nil {
		writeInboxError(w, err)
		return
	}
	s.modHub.Emit("queue.resumed", map[string]any{"source": "apk"})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) modQueueRetry(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := s.ctl().RetryInboxItem(id); err != nil {
		writeInboxError(w, err)
		return
	}
	s.modHub.Emit("queue.item.retried", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) modQueueRecoveryGet(w http.ResponseWriter, _ *http.Request) {
	snap := s.ctl().InboxSnapshot()
	writeJSON(w, map[string]any{"recovery": modQueueRecoverySummary(snap), "items": s.modQueueView()["items"]})
}

func (s *Server) modQueueRecoveryRetry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []string `json:"ids"`
		Resume bool     `json:"resume"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || len(body.IDs) == 0 || len(body.IDs) > sessioninbox.DefaultMaxItems {
		http.Error(w, "ids are required", http.StatusBadRequest)
		return
	}
	snap := s.ctl().InboxSnapshot()
	states := make(map[string]sessioninbox.InboxState, len(snap.Items))
	for _, item := range snap.Items {
		states[item.ID] = item.State
	}
	ids := make([]string, 0, len(body.IDs))
	seen := map[string]bool{}
	for _, raw := range body.IDs {
		id := strings.TrimSpace(raw)
		state, ok := states[id]
		if id == "" || !ok || (state != sessioninbox.StateUncertain && state != sessioninbox.StateBlocked) {
			http.Error(w, "every id must refer to an uncertain or blocked queue item", http.StatusConflict)
			return
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if err := s.ctl().RetryInboxItem(id); err != nil {
			writeInboxError(w, err)
			return
		}
	}
	if body.Resume {
		if err := s.ctl().SetInboxPaused(false); err != nil {
			writeInboxError(w, err)
			return
		}
	}
	s.modHub.Emit("queue.recovery.reviewed", map[string]any{"retried": len(ids), "resumed": body.Resume})
	writeJSON(w, map[string]any{"retried": ids, "resumed": body.Resume, "queue": s.modQueueView()})
}

func (s *Server) observeModInboxSnapshot(snap sessioninbox.InboxSnapshot) {
	if s == nil || s.modHub == nil {
		return
	}
	s.modHub.Emit("queue.updated", map[string]any{
		"revision": snap.Revision, "paused": snap.Paused, "recovered": snap.Recovered,
		"count": len(snap.Items), "stateCounts": modQueueStateCounts(snap.Items),
		"capacity": snap.Capacity, "recovery": modQueueRecoverySummary(snap),
	})
}
