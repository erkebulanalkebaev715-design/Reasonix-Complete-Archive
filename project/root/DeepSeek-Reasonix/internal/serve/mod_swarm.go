package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/swarm"
	"reasonix/internal/tool"
)

// modSwarmHost owns the Reasonix 3.0 swarm runtime behind the APK control
// plane. It reuses the real Reasonix provider/tool/agent path (nothing is
// simulated), streams typed swarm events through the existing broadcaster, and
// persists structured swarm state under the Reasonix home for recovery.
type modSwarmHost struct {
	mu     sync.Mutex
	store  *swarm.Store
	active *swarmRun
}

type swarmRun struct {
	state        *swarm.SwarmState
	orchestrator *swarm.Orchestrator
	cancel       context.CancelFunc
	startedAt    time.Time
}

func newModSwarmHost() *modSwarmHost {
	return &modSwarmHost{store: swarm.NewStore(config.SwarmStateDir())}
}

// modSwarmStartRequest is the APK-visible swarm start body.
type modSwarmStartRequest struct {
	Objective string                   `json:"objective"`
	Model     string                   `json:"model,omitempty"`
	Limits    *swarm.BudgetLimits      `json:"limits,omitempty"`
	Profiles  map[string]swarm.Profile `json:"profiles,omitempty"`
}

func (s *Server) modSwarmStart(w http.ResponseWriter, r *http.Request) {
	var req modSwarmStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Objective == "" {
		http.Error(w, "objective is required", http.StatusBadRequest)
		return
	}
	host := s.modSwarm
	if host == nil {
		http.Error(w, "swarm runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	host.mu.Lock()
	if host.active != nil {
		host.mu.Unlock()
		http.Error(w, "a swarm is already running; cancel it first", http.StatusConflict)
		return
	}
	host.mu.Unlock()

	base := tool.NewRegistry()
	for _, t := range tool.Builtins() {
		base.Add(t)
	}
	opts := []swarm.Option{
		swarm.WithSink(s.swarmSink()),
		swarm.WithPersister(host.store.Save),
	}
	if req.Limits != nil {
		opts = append(opts, swarm.WithLimits(*req.Limits))
	}
	if len(req.Profiles) > 0 {
		opts = append(opts, swarm.WithProfiles(req.Profiles))
	}
	resolver := s.modSwarmResolver
	if resolver == nil {
		resolver = swarm.DefaultConfigResolver()
	}
	o := swarm.NewOrchestrator(resolver, base, opts...)

	runCtx, cancel := context.WithCancel(context.Background())
	run := &swarmRun{orchestrator: o, cancel: cancel, startedAt: time.Now()}
	host.mu.Lock()
	host.active = run
	host.mu.Unlock()

	go func() {
		state, _ := o.Run(runCtx, req.Objective)
		host.mu.Lock()
		if host.active != nil && host.active.orchestrator == o {
			host.active.state = state
			host.active = nil
		}
		host.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted": true,
		"model":    req.Model,
	})
}

func (s *Server) modSwarmCancel(w http.ResponseWriter, _ *http.Request) {
	host := s.modSwarm
	if host == nil {
		http.Error(w, "swarm runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	host.mu.Lock()
	run := host.active
	host.mu.Unlock()
	if run == nil || run.orchestrator == nil {
		http.Error(w, "no active swarm", http.StatusNotFound)
		return
	}
	run.orchestrator.Cancel()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"cancelled": true})
}

func (s *Server) modSwarmGet(w http.ResponseWriter, _ *http.Request) {
	host := s.modSwarm
	if host == nil {
		http.Error(w, "swarm runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	host.mu.Lock()
	run := host.active
	host.mu.Unlock()
	if run != nil && run.orchestrator != nil {
		writeSwarmState(w, run.orchestrator.Snapshot())
		return
	}
	// No active swarm: fall back to the most recently persisted state so the
	// APK can read the last completed swarm without racing the background run.
	states, err := host.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(states) == 0 {
		http.Error(w, "no swarm has run yet", http.StatusNotFound)
		return
	}
	writeSwarmState(w, states[0])
}

func (s *Server) modSwarmGetByID(w http.ResponseWriter, r *http.Request) {
	host := s.modSwarm
	if host == nil {
		http.Error(w, "swarm runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	state, err := host.store.Load(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "swarm not found", http.StatusNotFound)
		return
	}
	writeSwarmState(w, state)
}

func (s *Server) modSwarmHistory(w http.ResponseWriter, _ *http.Request) {
	host := s.modSwarm
	if host == nil {
		http.Error(w, "swarm runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	states, err := host.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	limit := 20
	if len(states) > limit {
		states = states[:limit]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"swarms": states})
}

func writeSwarmState(w http.ResponseWriter, state *swarm.SwarmState) {
	if state == nil {
		http.Error(w, "no swarm state", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

// swarmSink fans typed swarm events into the server broadcaster so existing
// SSE /events subscribers see them alongside ordinary Reasonix events.
func (s *Server) swarmSink() event.Sink {
	return event.FuncSink(func(e event.Event) {
		if s.bc != nil {
			s.bc.Emit(e)
		}
	})
}
