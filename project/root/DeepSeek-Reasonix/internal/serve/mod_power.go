package serve

import (
	"context"
	"fmt"
	"net/http"

	"reasonix/internal/efficiency"
)

func (s *Server) modPowerGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"engine": s.modPower.Snapshot(), "turn": s.modPowerTurn.Snapshot(), "orchestrator": s.modAuto.Snapshot()})
}

func (s *Server) modPowerReset(w http.ResponseWriter, _ *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		http.Error(w, "cannot reset unified power state during automatic continuation", http.StatusConflict)
		return
	}
	if s.ctl().Running() {
		http.Error(w, "cannot reset unified power state while a turn is running", http.StatusConflict)
		return
	}
	snap := s.modPower.Reset(currentModelRef(s.ctl()))
	turn := s.modPowerTurn.Reset()
	auto := s.modAuto.Reset()
	s.modHub.Emit("power.reset", map[string]any{"engine": snap, "turn": turn, "orchestrator": auto})
	writeJSON(w, map[string]any{"engine": snap, "turn": turn, "orchestrator": auto})
}

func (s *Server) modPowerApplyPending(w http.ResponseWriter, r *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		http.Error(w, "automatic continuation already owns the pending route", http.StatusConflict)
		return
	}
	if s.ctl().Running() {
		http.Error(w, "cannot apply pending power route while a turn is running", http.StatusConflict)
		return
	}
	a, ok := s.modPowerTurn.Pending()
	if !ok {
		http.Error(w, "no pending power route", http.StatusConflict)
		return
	}
	res, err := s.applyUnifiedPowerAttemptCtx(r.Context(), a)
	if err != nil {
		s.modPowerTurn.Applied("blocked", false)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.modPowerTurn.Applied(string(res.Snapshot.NextAction), true)
	s.persistModAppStateBestEffort()
	writeJSON(w, map[string]any{"power": res.Snapshot, "turn": s.modPowerTurn.Snapshot()})
}

func (s *Server) applyUnifiedPowerAttempt(a efficiency.RepairAttempt) (efficiency.PowerResult, error) {
	return s.applyUnifiedPowerAttemptCtx(context.Background(), a)
}

func (s *Server) applyUnifiedPowerAttemptCtx(ctx context.Context, a efficiency.RepairAttempt) (efficiency.PowerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	beforeExec := s.modExec.Snapshot()
	res, err := s.modPower.Handle(ctx, efficiency.PowerAttempt{
		RepairAttempt:   a,
		CurrentModelRef: currentModelRef(s.ctl()),
	})
	snap := res.Snapshot
	s.modHub.Emit("power.updated", snap)
	s.modHub.Emit("repair.updated", snap.Cycle)
	s.modHub.Emit("router.decision", snap.Cycle.LastRoute)
	s.modHub.Emit("execution.updated", snap.Execution)
	if snap.Cycle.Recovery.Requested {
		kind := "rollback.failed"
		if snap.Cycle.Recovery.Succeeded {
			kind = "rollback.completed"
		}
		s.modHub.Emit(kind, snap.Cycle.Recovery)
	}
	if snap.Execution.Switches > beforeExec.Switches {
		s.modHub.Emit("model.switch.completed", map[string]any{
			"from": beforeExec.CurrentModelRef, "to": snap.Execution.CurrentModelRef, "mode": snap.Execution.Mode,
		})
		if snap.Execution.Mode == efficiency.ExecutionProDiagnosis {
			s.modHub.Emit("model.escalated", map[string]any{"to": snap.Execution.CurrentModelRef, "diagnosisOnly": true})
		}
		if snap.Execution.Mode == efficiency.ExecutionFlashRepair {
			s.modHub.Emit("model.returned_flash", map[string]any{"to": snap.Execution.CurrentModelRef})
		}
	}
	if err != nil {
		s.modHub.Emit("power.blocked", map[string]any{"reason": snap.Reason, "action": snap.NextAction})
		return res, fmt.Errorf("unified power engine: %w", err)
	}
	if snap.Cycle.State == "done" {
		s.modHub.Emit("repair.completed", map[string]any{"attempts": snap.Cycle.Attempts})
	}
	return res, nil
}
