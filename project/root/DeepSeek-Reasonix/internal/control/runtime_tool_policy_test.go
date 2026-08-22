package control

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/permission"
	"reasonix/internal/tool"
)

type runtimePolicyTestTool struct {
	name string
	ro   bool
}

func (t runtimePolicyTestTool) Name() string            { return t.name }
func (t runtimePolicyTestTool) Description() string     { return "test tool" }
func (t runtimePolicyTestTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t runtimePolicyTestTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "ok", nil
}
func (t runtimePolicyTestTool) ReadOnly() bool { return t.ro }

func TestSessionToolDecisionsUseNativePermissionGateAndHideDeniedSchema(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(runtimePolicyTestTool{name: "read_x", ro: true})
	reg.Add(runtimePolicyTestTool{name: "write_x"})
	c := New(Options{Registry: reg, Policy: permission.New("ask", nil, nil, nil)})
	c.EnableInteractiveApproval()
	if err := c.SetSessionToolDecisions(map[string]string{"read_x": "deny", "write_x": "allow"}); err != nil {
		t.Fatal(err)
	}
	if reg.ProviderVisible("read_x") {
		t.Fatal("denied tool must be hidden from provider schema")
	}
	if !reg.ProviderVisible("write_x") {
		t.Fatal("allowed tool should remain provider-visible")
	}
	gate := c.newInteractiveGate()
	allow, _, err := gate.Check(context.Background(), "read_x", json.RawMessage(`{}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if allow {
		t.Fatal("runtime deny did not reach native gate")
	}
	p := c.policyWithRuntimeToolDecisions(c.policy)
	if got := p.Decide("write_x", false, json.RawMessage(`{}`)); got != permission.Allow {
		t.Fatalf("write_x decision=%v", got)
	}
}

func TestSessionToolDecisionsRejectUnknownTool(t *testing.T) {
	c := New(Options{Registry: tool.NewRegistry()})
	if err := c.SetSessionToolDecisions(map[string]string{"ghost": "deny"}); err == nil {
		t.Fatal("expected unknown tool error")
	}
}
