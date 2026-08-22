package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/billing"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/efficiency"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestModBudgetAPIAndUsageAccounting(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	s := New(ctrl, bc, config.ServeConfig{})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// hardStop=false makes this test independent of whichever provider config is
	// present on the machine while still exercising the APK-facing KZT ledger.
	resp, err := http.Post(ts.URL+"/mod/budget", "application/json", strings.NewReader(`{
		"budgetKzt":1000,
		"reservePercent":15,
		"proMaxPercent":25,
		"hardStop":false,
		"fxKztPerUnit":{"USD":500}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /mod/budget = %d: %s", resp.StatusCode, body)
	}

	bc.Emit(event.Event{
		Kind:     event.Usage,
		ModelRef: "deepseek-pro/deepseek-v4-pro",
		Usage:    &provider.Usage{PromptTokens: 1, CompletionTokens: 1},
		CostQuote: &billing.CostQuote{
			Original:     billing.Money{Amount: "0.20", Currency: "USD"},
			CostComplete: true,
			ModelRef:     "deepseek-pro/deepseek-v4-pro",
		},
	})

	got, err := http.Get(ts.URL + "/mod/budget")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	var body struct {
		Budget struct {
			SpentKZT    float64 `json:"spentKzt"`
			ProSpentKZT float64 `json:"proSpentKzt"`
			Remaining   float64 `json:"remainingKzt"`
		} `json:"budget"`
	}
	if err := json.NewDecoder(got.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Budget.SpentKZT != 100 || body.Budget.ProSpentKZT != 100 || body.Budget.Remaining != 900 {
		t.Fatalf("budget after usage = %+v", body.Budget)
	}
}

func TestModBudgetRejectsUnknownFields(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	ts := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/mod/budget", "application/json", strings.NewReader(`{"budgetKzt":100,"wat":1}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestModStatusExposesTypedAPKSurface(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, ModelRef: "mock/flash"})
	ts := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/mod/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["modVersion"] != balanceModVersion || body["modelRef"] != "mock/flash" {
		t.Fatalf("status body = %#v", body)
	}
	if _, ok := body["budget"]; !ok {
		t.Fatal("status missing budget")
	}
	if _, ok := body["resources"]; !ok {
		t.Fatal("status missing resources")
	}
	if _, ok := body["quality"]; !ok {
		t.Fatal("status missing quality")
	}
	if _, ok := body["router"]; !ok {
		t.Fatal("status missing router")
	}
	if _, ok := body["cycle"]; !ok {
		t.Fatal("status missing repair cycle")
	}
	if _, ok := body["execution"]; !ok {
		t.Fatal("status missing execution router")
	}
}

func TestModQualityAPITracksLoopAndReadiness(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	ts := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer ts.Close()

	bc.Emit(event.Event{Kind: event.TurnStarted})
	bc.Emit(event.Event{Kind: event.Notice, Code: event.NoticeCodeLoopGuard})
	bc.Emit(event.Event{
		Kind:      event.TurnDone,
		Outcome:   event.TurnOutcomeFinalReadiness,
		Readiness: &event.FinalReadiness{Attempts: 1, Missing: []string{"verification"}},
	})

	resp, err := http.Get(ts.URL + "/mod/quality")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		State           string   `json:"state"`
		LoopGuardHits   int      `json:"loopGuardHits"`
		ReadinessBlocks int      `json:"readinessBlocks"`
		Missing         []string `json:"missing"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.State != "blocked" || body.LoopGuardHits != 1 || body.ReadinessBlocks != 1 {
		t.Fatalf("quality = %+v", body)
	}
	if len(body.Missing) != 1 || body.Missing[0] != "verification" {
		t.Fatalf("quality missing = %#v", body.Missing)
	}
}

func TestModRouterAPIIsAPKVisibleAndResettable(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	ts := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/mod/router")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /mod/router = %d", resp.StatusCode)
	}
	var before map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&before); err != nil {
		t.Fatal(err)
	}
	if _, ok := before["action"]; !ok {
		t.Fatal("router status missing action")
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mod/router/reset", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	reset, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer reset.Body.Close()
	if reset.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(reset.Body)
		t.Fatalf("POST /mod/router/reset = %d: %s", reset.StatusCode, body)
	}
}

func TestModCycleAPIAndHostRepairWiring(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, SessionDir: t.TempDir()})
	s := New(ctrl, bc, config.ServeConfig{})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	failed := efficiency.VerificationReceipt{RequiredChecks: 2, ChecksPassed: 1, ChecksFailed: 1, BuildObserved: true, BuildExitCode: 1}
	passed := efficiency.VerificationReceipt{RequiredChecks: 2, ChecksPassed: 2, BuildObserved: true, BuildExitCode: 0}

	steps := []efficiency.RepairAttempt{
		{StrategyID: "A", FailureFingerprint: "E1", PatchNumstat: "1\t0\ta.go", Verification: failed, EstimatedFlashKZT: 1, EstimatedProKZT: 5},
		{StrategyID: "B", FailureFingerprint: "E1", PatchNumstat: "1\t0\ta.go", Verification: failed, EstimatedFlashKZT: 1, EstimatedProKZT: 5},
		{StrategyID: "pro-diagnosis", FailureFingerprint: "E1", Verification: failed, EstimatedFlashKZT: 1},
		{StrategyID: "C", ResolvedFingerprint: "E1", FixHint: "verified repair", PatchNumstat: "1\t0\ta.go", Verification: passed, Finalization: true},
	}
	for _, step := range steps {
		if _, err := s.observeRepairAttempt(step); err != nil {
			t.Fatal(err)
		}
	}

	resp, err := http.Get(ts.URL + "/mod/cycle")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var snap efficiency.RepairCycleSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.State != "done" || snap.LastRoute.Action != efficiency.RouteFinalize || snap.Attempts != 4 {
		t.Fatalf("cycle=%+v", snap)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mod/cycle/reset", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	reset, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer reset.Body.Close()
	if reset.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(reset.Body)
		t.Fatalf("reset=%d: %s", reset.StatusCode, body)
	}
}

func TestModRecoveryAPIIsFailClosedWithoutCheckpoint(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, SessionDir: t.TempDir()})
	ts := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/mod/recovery")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var status modRecoveryStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Available || status.CheckpointCount != 0 {
		t.Fatalf("recovery=%+v", status)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mod/recovery/rollback-last", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rollback, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Body.Close()
	if rollback.StatusCode != http.StatusConflict {
		t.Fatalf("rollback status=%d, want 409", rollback.StatusCode)
	}
}

func TestModExecutionAPIIsStrictAndAPKVisible(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, ModelRef: "mock/flash-a"})
	s := New(ctrl, bc, config.ServeConfig{})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Unknown refs are rejected instead of being saved for a later surprise
	// provider/network failure.
	bad, err := http.Post(ts.URL+"/mod/execution/config", "application/json", strings.NewReader(`{
		"enabled":true,
		"flashPrimaryRef":"mock/flash-a",
		"proRef":"missing/pro"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown execution ref status=%d, want 400", bad.StatusCode)
	}

	// The current ref is always a valid local selection, so this exercises the
	// APK contract without depending on machine-global provider config.
	okResp, err := http.Post(ts.URL+"/mod/execution/config", "application/json", strings.NewReader(`{
		"enabled":true,
		"flashPrimaryRef":"mock/flash-a",
		"flashAlternativeRef":"mock/flash-a",
		"proRef":"mock/flash-a",
		"flashRepairRef":"mock/flash-a"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer okResp.Body.Close()
	if okResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(okResp.Body)
		t.Fatalf("execution config status=%d: %s", okResp.StatusCode, body)
	}

	get, err := http.Get(ts.URL + "/mod/execution")
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var snap efficiency.ExecutionSnapshot
	if err := json.NewDecoder(get.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if !snap.Enabled || snap.Config.FlashPrimaryRef != "mock/flash-a" {
		t.Fatalf("execution=%+v", snap)
	}
}

func TestModExecutionRouterUsesNativeServeModelSwitch(t *testing.T) {
	bc := NewBroadcaster()
	dir := t.TempDir()
	ctrl := control.New(control.Options{Sink: bc, ModelRef: "mock/flash-a", SessionDir: dir})
	s := New(ctrl, bc, config.ServeConfig{})
	defer func() { s.ctl().Close() }()

	cfg := efficiency.ExecutionConfig{
		Enabled:             true,
		FlashPrimaryRef:     "mock/flash-a",
		FlashAlternativeRef: "mock/flash-b",
		ProRef:              "mock/pro",
		FlashRepairRef:      "mock/flash-repair",
	}
	if _, err := s.modExec.Configure(cfg, currentModelRef(s.ctl())); err != nil {
		t.Fatal(err)
	}

	var switched []string
	s.buildController = func(_ context.Context, ref string) (*control.Controller, error) {
		switched = append(switched, ref)
		return control.New(control.Options{Sink: bc, ModelRef: ref, SessionDir: dir}), nil
	}

	failed := efficiency.VerificationReceipt{RequiredChecks: 1, ChecksFailed: 1, BuildObserved: true, BuildExitCode: 1}
	passed := efficiency.VerificationReceipt{RequiredChecks: 1, ChecksPassed: 1, BuildObserved: true, BuildExitCode: 0}
	steps := []efficiency.RepairAttempt{
		{StrategyID: "A", FailureFingerprint: "E1", Verification: failed, EstimatedFlashKZT: 1, EstimatedProKZT: 5},
		{StrategyID: "B", FailureFingerprint: "E1", Verification: failed, EstimatedFlashKZT: 1, EstimatedProKZT: 5},
		{StrategyID: "pro-diagnosis", FailureFingerprint: "E1", Verification: failed, EstimatedFlashKZT: 1},
		{StrategyID: "repair", ResolvedFingerprint: "E1", Verification: passed, Finalization: true},
	}
	for _, step := range steps {
		if _, err := s.observeRepairAttemptCtx(context.Background(), step); err != nil {
			t.Fatalf("step %s: %v", step.StrategyID, err)
		}
	}
	want := []string{"mock/flash-b", "mock/pro", "mock/flash-repair"}
	if !reflect.DeepEqual(switched, want) {
		t.Fatalf("native switches=%v want=%v", switched, want)
	}
	if got := currentModelRef(s.ctl()); got != "mock/flash-repair" {
		t.Fatalf("current model=%q", got)
	}
	execSnap := s.modExec.Snapshot()
	if execSnap.Mode != efficiency.ExecutionFinished || execSnap.Switches != 3 {
		t.Fatalf("execution=%+v", execSnap)
	}
}
