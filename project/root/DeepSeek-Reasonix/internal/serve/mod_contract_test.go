package serve

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestModAPKContractFrozenV1AndBootstrapNegotiates(t *testing.T) {
	ts, _ := newModAgentTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/mod/app/contract")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("contract status=%d", resp.StatusCode)
	}
	var c modAPKContract
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	if c.ProtocolVersion != "balance-apk-v1" || c.ContractRevision != 1 {
		t.Fatalf("protocol=%q revision=%d", c.ProtocolVersion, c.ContractRevision)
	}
	if len(c.Endpoints) != 72 || len(c.Events) != 85 {
		t.Fatalf("frozen v1 surface changed: endpoints=%d events=%d; make the compatibility decision explicit", len(c.Endpoints), len(c.Events))
	}
	if len(c.Digest) != 64 || c.Digest != modAPKContractDigest() {
		t.Fatalf("digest=%q", c.Digest)
	}
	if exported, _ := c.Guarantees["hiddenReasoningExported"].(bool); exported {
		t.Fatal("contract must never advertise hidden reasoning export")
	}
	seenEndpoint := map[string]modContractEndpoint{}
	for _, e := range c.Endpoints {
		seenEndpoint[e.Name] = e
	}
	for name, want := range map[string]string{
		"bootstrap": "/mod/app/bootstrap", "contract": "/mod/app/contract", "apply": "/mod/app/apply",
		"events": "/mod/events", "capabilities": "/mod/capabilities", "queue": "/mod/queue",
	} {
		if seenEndpoint[name].Path != want {
			t.Fatalf("endpoint %s=%+v", name, seenEndpoint[name])
		}
	}
	eventSet := map[string]bool{}
	for _, e := range c.Events {
		eventSet[e] = true
	}
	for _, want := range []string{"live.turn.started", "live.tool.started", "live.tool.finished", "live.verification.summary", "live.turn.done", "budget.updated", "model.escalated"} {
		if !eventSet[want] {
			t.Fatalf("missing event %q", want)
		}
	}
	for e := range eventSet {
		if strings.Contains(strings.ToLower(e), "reasoning") || strings.Contains(strings.ToLower(e), "chain") {
			t.Fatalf("reasoning-like event leaked into frozen contract: %q", e)
		}
	}

	boot, err := http.Get(ts.URL + "/mod/app/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	defer boot.Body.Close()
	var b map[string]any
	if err := json.NewDecoder(boot.Body).Decode(&b); err != nil {
		t.Fatal(err)
	}
	summary, ok := b["contract"].(map[string]any)
	if !ok {
		t.Fatalf("bootstrap contract=%T %v", b["contract"], b["contract"])
	}
	if summary["digest"] != c.Digest || summary["endpoint"] != "/mod/app/contract" {
		t.Fatalf("bootstrap contract summary=%v full=%s", summary, c.Digest)
	}
}

func TestModAPKContractRequiresJSONForPOSTByPolicy(t *testing.T) {
	c := modAPKContractPayload()
	if c.RequestRules["mutatingRequestsContentType"] != "application/json" {
		t.Fatalf("request rules=%v", c.RequestRules)
	}
	for _, e := range c.Endpoints {
		if e.Method == http.MethodPost && !e.JSONBody {
			t.Fatalf("POST endpoint missing JSON-body contract: %+v", e)
		}
	}
}
