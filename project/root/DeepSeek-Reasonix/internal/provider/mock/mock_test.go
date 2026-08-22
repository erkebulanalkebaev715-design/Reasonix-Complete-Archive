package mock

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func collect(t *testing.T, p provider.Provider, req provider.Request) []provider.Chunk {
	t.Helper()
	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var out []provider.Chunk
	for c := range ch {
		out = append(out, c)
	}
	return out
}

func TestSmokeScenarioAdvancesByToolResults(t *testing.T) {
	p, err := New(provider.Config{Name: "mock", Extra: map[string]any{"scenario": "smoke", "path": "hello.py"}})
	if err != nil {
		t.Fatal(err)
	}
	tools := []provider.ToolSchema{{Name: "read_file"}, {Name: "use_capability"}}
	first := collect(t, p, provider.Request{Tools: tools})
	if len(first) == 0 || first[0].Type != provider.ChunkToolCall || first[0].ToolCall.Name != "read_file" {
		t.Fatalf("first = %#v", first)
	}
	second := collect(t, p, provider.Request{Tools: tools, Messages: []provider.Message{{Role: provider.RoleTool}}})
	if len(second) == 0 || second[0].Type != provider.ChunkToolCall || second[0].ToolCall.Name != "use_capability" {
		t.Fatalf("second = %#v", second)
	}
	if !strings.Contains(second[0].ToolCall.Arguments, `"capability_id":"tool:grep"`) {
		t.Fatalf("second args = %q", second[0].ToolCall.Arguments)
	}
	third := collect(t, p, provider.Request{Tools: tools, Messages: []provider.Message{{Role: provider.RoleTool}, {Role: provider.RoleTool}}})
	if len(third) == 0 || third[0].Type != provider.ChunkText || third[0].Text != "OFFLINE_MOCK_PASS" {
		t.Fatalf("third = %#v", third)
	}
}

func TestRepeatFailureAlwaysRequestsMissingRead(t *testing.T) {
	p, err := New(provider.Config{Name: "mock", Extra: map[string]any{"scenario": "repeat-failure"}})
	if err != nil {
		t.Fatal(err)
	}
	chunks := collect(t, p, provider.Request{Tools: []provider.ToolSchema{{Name: "read_file"}}, Messages: []provider.Message{{Role: provider.RoleTool}}})
	if len(chunks) == 0 || chunks[0].ToolCall == nil || chunks[0].ToolCall.Name != "read_file" {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestRepeatFailureStopsAfterLoopGuardInstruction(t *testing.T) {
	p, err := New(provider.Config{Name: "mock", Extra: map[string]any{"scenario": "repeat-failure"}})
	if err != nil {
		t.Fatal(err)
	}
	chunks := collect(t, p, provider.Request{
		Tools:    []provider.ToolSchema{{Name: "read_file"}},
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "[loop guard] Change approach now."}},
	})
	if len(chunks) == 0 || chunks[0].Type != provider.ChunkText || chunks[0].Text != "OFFLINE_LOOP_GUARD_PASS" {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestRepairCycleScenarioSequence(t *testing.T) {
	p, err := New(provider.Config{Name: "mock", Extra: map[string]any{"scenario": "repair-cycle"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"MOCK_REPAIR_FAIL_A", "MOCK_REPAIR_FAIL_B", "MOCK_PRO_DIAGNOSIS", "MOCK_REPAIR_PASS"}
	for i, expected := range want {
		chunks := collect(t, p, provider.Request{})
		if len(chunks) == 0 || chunks[0].Type != provider.ChunkText || chunks[0].Text != expected {
			t.Fatalf("step %d = %#v, want %s", i+1, chunks, expected)
		}
	}
}

func TestDenyBypassScenarioRequiresSchemaTrimAndNativeDenial(t *testing.T) {
	p, err := New(provider.Config{Name: "mock", Extra: map[string]any{"scenario": "deny-bypass"}})
	if err != nil {
		t.Fatal(err)
	}
	// A direct writer in the provider schema is already a policy failure.
	bad := collect(t, p, provider.Request{Tools: []provider.ToolSchema{{Name: "write_file"}, {Name: "use_capability"}}})
	if len(bad) == 0 || bad[0].Type != provider.ChunkText || bad[0].Text != "MOCK_DENY_SCHEMA_FAIL" {
		t.Fatalf("bad schema result = %#v", bad)
	}

	p, err = New(provider.Config{Name: "mock", Extra: map[string]any{"scenario": "deny-bypass"}})
	if err != nil {
		t.Fatal(err)
	}
	first := collect(t, p, provider.Request{Tools: []provider.ToolSchema{{Name: "use_capability"}}})
	if len(first) == 0 || first[0].ToolCall == nil || first[0].ToolCall.Name != "use_capability" {
		t.Fatalf("proxy attempt = %#v", first)
	}
	if !strings.Contains(first[0].ToolCall.Arguments, `"capability_id":"tool:write_file"`) {
		t.Fatalf("proxy args = %q", first[0].ToolCall.Arguments)
	}
	second := collect(t, p, provider.Request{
		Tools:    []provider.ToolSchema{{Name: "use_capability"}},
		Messages: []provider.Message{{Role: provider.RoleTool, Name: "use_capability", Content: "blocked: denied by permission policy — do not retry"}},
	})
	if len(second) == 0 || second[0].Type != provider.ChunkText || second[0].Text != "OFFLINE_DENY_NATIVE_PASS" {
		t.Fatalf("denied result = %#v", second)
	}
}

func TestBudgetCapScenarioRequiresExplicitProviderOutputCap(t *testing.T) {
	p, err := New(provider.Config{Name: "mock", Extra: map[string]any{"scenario": "budget-cap"}})
	if err != nil {
		t.Fatal(err)
	}
	bad := collect(t, p, provider.Request{})
	if len(bad) == 0 || bad[0].Type != provider.ChunkText || !strings.Contains(bad[0].Text, "MOCK_PRECALL_BUDGET_FAIL") {
		t.Fatalf("uncapped request = %#v", bad)
	}
	good := collect(t, p, provider.Request{MaxTokens: 4096})
	if len(good) == 0 || good[0].Type != provider.ChunkText || good[0].Text != "OFFLINE_PRECALL_BUDGET_PASS" {
		t.Fatalf("capped request = %#v", good)
	}
}
