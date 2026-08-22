// Package mock provides a deterministic, offline provider used to validate the
// Balance Mod and Reasonix's tool loop without an API key or network access.
package mock

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"reasonix/internal/provider"
)

const Kind = "mock"

type mockProvider struct {
	name     string
	scenario string
	path     string
	mu       sync.Mutex
	step     int
}

func init() {
	provider.Register(Kind, New)
}

// New builds an offline mock provider. Supported Extra fields:
//
//	scenario: smoke (default) | repeat-failure | repair-cycle | deny-bypass | budget-cap | text
//	path: file used by smoke/repeat-failure (default hello.py)
func New(cfg provider.Config) (provider.Provider, error) {
	scenario := extraString(cfg.Extra, "scenario")
	if scenario == "" {
		candidate := strings.ToLower(strings.TrimSpace(cfg.Model))
		switch candidate {
		case "smoke", "repeat-failure", "repair-cycle", "deny-bypass", "budget-cap", "text":
			scenario = candidate
		default:
			scenario = "smoke"
		}
	}
	switch scenario {
	case "smoke", "repeat-failure", "repair-cycle", "deny-bypass", "budget-cap", "text":
	default:
		return nil, fmt.Errorf("mock provider: unknown scenario %q", scenario)
	}
	path := extraString(cfg.Extra, "path")
	if path == "" {
		if body, ok := cfg.Extra["extra_body"].(map[string]any); ok {
			if v, ok := body["mock_path"].(string); ok {
				path = strings.TrimSpace(v)
			}
		}
	}
	if path == "" {
		path = "hello.py"
	}
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = "balance-mock"
	}
	return &mockProvider{name: name, scenario: scenario, path: path}, nil
}

func (p *mockProvider) Name() string { return p.name }

// OutputBudget gives the budget-cap scenario a deliberately large baseline so
// the v0.16 process test can prove the host actually reduced the provider
// request before I/O rather than merely observing an already-small default.
func (p *mockProvider) OutputBudget() int {
	if p != nil && p.scenario == "budget-cap" {
		return provider.DefaultHighOutputTokens
	}
	return 0
}

func (p *mockProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk, 4)
	go func() {
		defer close(ch)
		emit := func(c provider.Chunk) bool {
			select {
			case ch <- c:
				return true
			case <-ctx.Done():
				return false
			}
		}

		toolResults := countToolResults(req.Messages)
		calledTool := false
		switch p.scenario {
		case "budget-cap":
			// v0.16 process gate: a hard Balance budget must impose an explicit
			// provider output ceiling before this offline provider is called.
			// Zero means the request escaped the pre-call guard.
			if req.MaxTokens > 0 && req.MaxTokens < provider.DefaultHighOutputTokens {
				if !emit(provider.Chunk{Type: provider.ChunkText, Text: "OFFLINE_PRECALL_BUDGET_PASS"}) {
					return
				}
			} else if !emit(provider.Chunk{Type: provider.ChunkText, Text: fmt.Sprintf("MOCK_PRECALL_BUDGET_FAIL max_tokens=%d", req.MaxTokens)}) {
				return
			}
		case "deny-bypass":
			// Stress the real policy boundary, not only provider-schema trimming.
			// A denied write_file should be absent from the provider schema; the
			// mock then deliberately tries the stable use_capability proxy. The
			// resolved target must still be rejected by Reasonix's native gate.
			if hasTool(req.Tools, "write_file") {
				if !emit(provider.Chunk{Type: provider.ChunkText, Text: "MOCK_DENY_SCHEMA_FAIL"}) {
					return
				}
				break
			}
			if toolResults == 0 {
				if hasTool(req.Tools, "use_capability") {
					if !emit(provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
						ID:        "mock-deny-proxy-1",
						Name:      "use_capability",
						Arguments: `{"action":"call","capability_id":"tool:write_file","arguments":{"path":"should_not_exist.txt","content":"DENY_BYPASS_SHOULD_NEVER_WRITE"}}`,
					}}) {
						return
					}
					calledTool = true
				} else if !emit(provider.Chunk{Type: provider.ChunkText, Text: "MOCK_DENY_NO_PROXY"}) {
					return
				}
				break
			}
			if messagesContain(req.Messages, "denied by permission policy") || messagesContain(req.Messages, "blocked:") {
				if !emit(provider.Chunk{Type: provider.ChunkText, Text: "OFFLINE_DENY_NATIVE_PASS"}) {
					return
				}
			} else if !emit(provider.Chunk{Type: provider.ChunkText, Text: "MOCK_DENY_NATIVE_FAIL"}) {
				return
			}
		case "repair-cycle":
			p.mu.Lock()
			p.step++
			step := p.step
			p.mu.Unlock()
			text := "MOCK_REPAIR_PASS"
			switch step {
			case 1:
				text = "MOCK_REPAIR_FAIL_A"
			case 2:
				text = "MOCK_REPAIR_FAIL_B"
			case 3:
				text = "MOCK_PRO_DIAGNOSIS"
			}
			if !emit(provider.Chunk{Type: provider.ChunkText, Text: text}) {
				return
			}
		case "repeat-failure":
			// Keep repeating the same failing read until Reasonix's native loop
			// guard injects a change-approach instruction. The mock then stops,
			// proving the host intervention reached the provider-visible history.
			if messagesContain(req.Messages, "[loop guard]") {
				if !emit(provider.Chunk{Type: provider.ChunkText, Text: "OFFLINE_LOOP_GUARD_PASS"}) {
					return
				}
				break
			}
			path := p.path
			if path == "hello.py" {
				path = "__balance_mod_missing__.txt"
			}
			if hasTool(req.Tools, "read_file") {
				if !emit(provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
					ID:        fmt.Sprintf("mock-read-fail-%d", toolResults+1),
					Name:      "read_file",
					Arguments: fmt.Sprintf(`{"path":%q}`, path),
				}}) {
					return
				}
				calledTool = true
			} else {
				if !emit(provider.Chunk{Type: provider.ChunkText, Text: "MOCK_NO_READ_FILE_TOOL"}) {
					return
				}
			}
		case "text":
			if !emit(provider.Chunk{Type: provider.ChunkText, Text: "OFFLINE_MOCK_TEXT_PASS"}) {
				return
			}
		default: // smoke
			switch toolResults {
			case 0:
				if hasTool(req.Tools, "read_file") {
					if !emit(provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
						ID:        "mock-read-1",
						Name:      "read_file",
						Arguments: fmt.Sprintf(`{"path":%q}`, p.path),
					}}) {
						return
					}
					calledTool = true
				} else if !emit(provider.Chunk{Type: provider.ChunkText, Text: "MOCK_NO_READ_FILE_TOOL"}) {
					return
				}
			case 1:
				// Reasonix keeps optional tools such as grep behind the stable
				// provider-visible use_capability proxy. Prefer direct grep when a
				// custom surface exposes it, otherwise exercise the real proxy path.
				if hasTool(req.Tools, "grep") {
					if !emit(provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
						ID:        "mock-grep-2",
						Name:      "grep",
						Arguments: fmt.Sprintf(`{"pattern":"TEST OK","path":%q}`, p.path),
					}}) {
						return
					}
					calledTool = true
				} else if hasTool(req.Tools, "use_capability") {
					if !emit(provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
						ID:   "mock-grep-proxy-2",
						Name: "use_capability",
						Arguments: fmt.Sprintf(
							`{"action":"call","capability_id":"tool:grep","arguments":{"pattern":"TEST OK","path":%q}}`,
							p.path,
						),
					}}) {
						return
					}
					calledTool = true
				} else if !emit(provider.Chunk{Type: provider.ChunkText, Text: "MOCK_NO_SEARCH_TOOL"}) {
					return
				}
			default:
				if !emit(provider.Chunk{Type: provider.ChunkText, Text: "OFFLINE_MOCK_PASS"}) {
					return
				}
			}
		}

		finishReason := "stop"
		if calledTool {
			finishReason = "tool_calls"
		}
		if !emit(provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{
			PromptTokens:     16,
			CompletionTokens: 4,
			TotalTokens:      20,
			CacheMissTokens:  16,
			FinishReason:     finishReason,
			RequestCount:     1,
		}}) {
			return
		}
		emit(provider.Chunk{Type: provider.ChunkDone})
	}()
	return ch, nil
}

func countToolResults(messages []provider.Message) int {
	n := 0
	for _, m := range messages {
		if m.Role == provider.RoleTool {
			n++
		}
	}
	return n
}

func hasTool(tools []provider.ToolSchema, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func extraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	v, _ := extra[key].(string)
	return strings.TrimSpace(v)
}

func messagesContain(messages []provider.Message, needle string) bool {
	for _, m := range messages {
		if strings.Contains(m.Content, needle) {
			return true
		}
	}
	return false
}
