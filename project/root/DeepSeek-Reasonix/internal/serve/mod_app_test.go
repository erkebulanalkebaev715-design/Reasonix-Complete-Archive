package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/billing"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/efficiency"
	"reasonix/internal/event"
	"reasonix/internal/memory"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func newPersistentModServer(t *testing.T, root string) (*httptest.Server, *Broadcaster) {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(modAgentTestTool{name: "read_x", ro: true})
	reg.Add(modAgentTestTool{name: "write_x"})
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, Registry: reg, WorkspaceRoot: root, SessionDir: filepath.Join(root, ".sessions"), Memory: memory.Load(memory.Options{CWD: root, UserDir: ""}), ModelRef: "mock/flash"})
	return httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler()), bc
}

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestModAppBootstrapAndAtomicApply(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ts, _ := newPersistentModServer(t, root)
	defer ts.Close()

	apply := postJSON(t, ts.URL+"/mod/app/apply", `{
      "profile":{"name":"Game","mode":"chat","toolPacks":["developer"],"liveDetail":"project"},
      "budget":{"budgetKzt":1000,"reservePercent":15,"proMaxPercent":25,"hardStop":false,"fxKztPerUnit":{"KZT":1}},
      "toolDecisions":{"read_x":"deny"},"approvalMode":"ask"
    }`)
	raw, _ := io.ReadAll(apply.Body)
	apply.Body.Close()
	if apply.StatusCode != http.StatusOK {
		t.Fatalf("apply=%d %s", apply.StatusCode, raw)
	}
	if !strings.Contains(string(raw), `"protocolVersion":"balance-apk-v1"`) {
		t.Fatalf("bootstrap missing protocol: %s", raw)
	}
	if !strings.Contains(string(raw), `"name":"Game"`) {
		t.Fatalf("profile missing: %s", raw)
	}

	stop := postJSON(t, ts.URL+"/mod/app/task/stop", `{}`)
	stop.Body.Close()
	if stop.StatusCode != http.StatusNoContent {
		t.Fatalf("stop alias=%d", stop.StatusCode)
	}
	badStart := postJSON(t, ts.URL+"/mod/app/task/start", `{}`)
	badStart.Body.Close()
	if badStart.StatusCode != http.StatusBadRequest {
		t.Fatalf("start alias should reuse native submit validation, got %d", badStart.StatusCode)
	}
}

func TestModAppStatePersistsBudgetSpendProfileAndToolsAcrossRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ts1, bc := newPersistentModServer(t, root)
	apply := postJSON(t, ts1.URL+"/mod/app/apply", `{
      "profile":{"name":"Persisted","mode":"agent","toolPacks":["developer"],"liveDetail":"metadata"},
      "budget":{"budgetKzt":100,"reservePercent":10,"proMaxPercent":25,"hardStop":false,"fxKztPerUnit":{"KZT":1}},
      "toolDecisions":{"write_x":"deny"},"approvalMode":"ask"
    }`)
	io.Copy(io.Discard, apply.Body)
	apply.Body.Close()
	if apply.StatusCode != http.StatusOK {
		t.Fatalf("apply=%d", apply.StatusCode)
	}
	bc.Emit(event.Event{Kind: event.Usage, ModelRef: "mock/flash", Usage: &provider.Usage{PromptTokens: 1}, CostQuote: &billing.CostQuote{Original: billing.Money{Amount: "40", Currency: "KZT"}, CostComplete: true, ModelRef: "mock/flash"}})
	ts1.Close()

	ts2, _ := newPersistentModServer(t, root)
	defer ts2.Close()
	resp, err := http.Get(ts2.URL + "/mod/app/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Profile struct {
			Name string `json:"name"`
		} `json:"profile"`
		Budget struct {
			SpentKZT  float64 `json:"spentKzt"`
			BudgetKZT float64 `json:"budgetKzt"`
		} `json:"budget"`
		Tools       []modToolView  `json:"tools"`
		Persistence map[string]any `json:"persistence"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Profile.Name != "Persisted" {
		t.Fatalf("profile=%+v", got.Profile)
	}
	if got.Budget.SpentKZT != 40 || got.Budget.BudgetKZT != 100 {
		t.Fatalf("budget=%+v", got.Budget)
	}
	seen := map[string]modToolView{}
	for _, v := range got.Tools {
		seen[v.Name] = v
	}
	if seen["write_x"].Decision != "deny" || seen["write_x"].ProviderVisible {
		t.Fatalf("tool policy not restored: %+v", seen["write_x"])
	}
	if enabled, _ := got.Persistence["enabled"].(bool); !enabled {
		t.Fatalf("persistence=%v", got.Persistence)
	}
}

// Compile-time guard: the test tool still satisfies the actual native tool contract.
var _ tool.Tool = modAgentTestTool{}
var _ = context.Background

func TestModPendingPowerRoutePersistsSanitizedAcrossRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := t.TempDir()
	reg := tool.NewRegistry()
	reg.Add(modAgentTestTool{name: "read_x", ro: true})
	bc1 := NewBroadcaster()
	ctrl1 := control.New(control.Options{Sink: bc1, Registry: reg, WorkspaceRoot: root, SessionDir: filepath.Join(root, ".sessions"), ModelRef: "mock/flash"})
	s1 := New(ctrl1, bc1, config.ServeConfig{})
	a := efficiency.RepairAttempt{
		StrategyID: "flash-a", FailureFingerprint: "fp-1", PatchNumstat: "1\t1\tx.go",
		BuildLog: "SECRET_RAW_BUILD_LOG", FixHint: "SECRET_FIX_HINT",
		Verification: efficiency.VerificationReceipt{RequiredChecks: 1, ChecksFailed: 1},
	}
	s1.modPowerTurn.RestorePending("pending-stable-id", a)
	if err := s1.persistModAppState(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s1.modPersistPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET_RAW_BUILD_LOG") || strings.Contains(string(raw), "SECRET_FIX_HINT") {
		t.Fatalf("content-bearing repair fields leaked into durable APK state: %s", raw)
	}

	bc2 := NewBroadcaster()
	ctrl2 := control.New(control.Options{Sink: bc2, Registry: reg, WorkspaceRoot: root, SessionDir: filepath.Join(root, ".sessions"), ModelRef: "mock/flash"})
	s2 := New(ctrl2, bc2, config.ServeConfig{})
	id, restored, ok := s2.modPowerTurn.PendingRecord()
	if !ok || id != "pending-stable-id" || restored.FailureFingerprint != "fp-1" {
		t.Fatalf("restored=(%q,%+v,%v)", id, restored, ok)
	}
	if restored.BuildLog != "" || restored.FixHint != "" {
		t.Fatal("restored durable route must remain sanitized")
	}
	if snap := s2.modAuto.Snapshot(); !snap.PendingRoute || snap.Active {
		t.Fatalf("restart must require explicit APK resume: %+v", snap)
	}
}
