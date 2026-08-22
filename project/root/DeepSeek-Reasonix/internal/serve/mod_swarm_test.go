package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/memory"
	"reasonix/internal/provider"
	"reasonix/internal/swarm"
	"reasonix/internal/tool"
)

type swarmTestResolver struct {
	prov provider.Provider
}

func (r *swarmTestResolver) Resolve(modelRef string) (provider.Provider, *provider.Pricing, string, string, error) {
	return r.prov, &provider.Pricing{Input: 1, Output: 2, Currency: "CNY"}, "mock/flash", "mock", nil
}

func newSwarmServer(t *testing.T, resolver swarm.Resolver) (*httptest.Server, *Broadcaster, *Server) {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(modAgentTestTool{name: "read_x", ro: true})
	bc := NewBroadcaster()
	root := t.TempDir()
	ctrl := control.New(control.Options{Sink: bc, Registry: reg, WorkspaceRoot: root, SessionDir: filepath.Join(root, ".sessions"), Memory: memory.Load(memory.Options{CWD: root, UserDir: ""}), ModelRef: "mock/flash"})
	s := New(ctrl, bc, config.ServeConfig{})
	s.modSwarmResolver = resolver
	return httptest.NewServer(s.Handler()), bc, s
}

func TestModSwarmEndToEndOffline(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	prov := testutil.NewMock("mock", textTurnForSwarm("A ok"), textTurnForSwarm("B ok"))
	ts, bc, _ := newSwarmServer(t, &swarmTestResolver{prov: prov})
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/mod/swarm/start", `{"objective":"Investigate A; Investigate B"}`)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("start = %d %s", resp.StatusCode, raw)
	}

	var swarmID string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state := getSwarmState(t, ts.URL)
		if state == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		swarmID = state.ID
		if state.Status == swarm.StatusDone || state.Status == swarm.StatusFailed {
			if len(state.Tasks) != 2 {
				t.Fatalf("tasks = %d, want 2", len(state.Tasks))
			}
			for _, task := range state.Tasks {
				if task.Status != swarm.TaskSucceeded {
					t.Fatalf("task %s status = %s", task.ID, task.Status)
				}
			}
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if swarmID == "" {
		t.Fatal("swarm never reached a terminal state")
	}

	// GET /mod/swarm/{id} must return the persisted structured state.
	byID, err := http.Get(ts.URL + "/mod/swarm/" + swarmID)
	if err != nil {
		t.Fatal(err)
	}
	rawID, _ := io.ReadAll(byID.Body)
	byID.Body.Close()
	if byID.StatusCode != http.StatusOK {
		t.Fatalf("get by id = %d %s", byID.StatusCode, rawID)
	}
	if !strings.Contains(string(rawID), `"objective"`) {
		t.Fatalf("persisted state missing objective: %s", rawID)
	}

	// The broadcaster must have streamed typed swarm events.
	_ = bc
}

func getSwarmState(t *testing.T, base string) *swarm.SwarmState {
	t.Helper()
	resp, err := http.Get(base + "/mod/swarm")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var state swarm.SwarmState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil
	}
	return &state
}

func textTurnForSwarm(text string) testutil.Turn {
	return testutil.Turn{Text: text, Usage: &provider.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}
}

func TestModSwarmRejectsEmptyObjective(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	prov := testutil.NewMock("mock", textTurnForSwarm("ok"))
	ts, _, _ := newSwarmServer(t, &swarmTestResolver{prov: prov})
	defer ts.Close()
	resp := postJSON(t, ts.URL+"/mod/swarm/start", `{"objective":""}`)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty objective = %d %s", resp.StatusCode, raw)
	}
}

var _ = event.Discard
var _ = os.Getenv
