package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type hiddenTestTool struct{ name string }

func (t hiddenTestTool) Name() string                                             { return t.name }
func (t hiddenTestTool) Description() string                                      { return "x" }
func (t hiddenTestTool) Schema() json.RawMessage                                  { return json.RawMessage(`{"type":"object"}`) }
func (t hiddenTestTool) Execute(context.Context, json.RawMessage) (string, error) { return "", nil }
func (t hiddenTestTool) ReadOnly() bool                                           { return true }
func TestSessionHiddenToolsOnlyTrimProviderSchema(t *testing.T) {
	r := NewRegistry()
	r.Add(hiddenTestTool{name: "a"})
	r.Add(hiddenTestTool{name: "b"})
	r.SetSessionHiddenTools([]string{"a"})
	if r.ProviderVisible("a") {
		t.Fatal("a visible")
	}
	if _, ok := r.Get("a"); !ok {
		t.Fatal("hidden tool must remain resolvable for permission-gated capability path")
	}
	if got := len(r.Schemas()); got != 1 {
		t.Fatalf("schemas=%d", got)
	}
	r.SetSessionHiddenTools(nil)
	if !r.ProviderVisible("a") {
		t.Fatal("clear hidden failed")
	}
}
