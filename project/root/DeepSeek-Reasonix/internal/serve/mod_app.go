package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/efficiency"
)

const modAppStateSchema = 1
const modAPKProtocolVersion = "balance-apk-v1"

type modPersistedPowerRoute struct {
	ID                  string                         `json:"id"`
	StrategyID          string                         `json:"strategyId,omitempty"`
	FailureFingerprint  string                         `json:"failureFingerprint,omitempty"`
	ResolvedFingerprint string                         `json:"resolvedFingerprint,omitempty"`
	PatchNumstat        string                         `json:"patchNumstat,omitempty"`
	Regression          bool                           `json:"regression,omitempty"`
	Verification        efficiency.VerificationReceipt `json:"verification"`
	EstimatedFlashKZT   float64                        `json:"estimatedFlashKzt,omitempty"`
	EstimatedProKZT     float64                        `json:"estimatedProKzt,omitempty"`
	Finalization        bool                           `json:"finalization,omitempty"`
}

func sanitizePersistedNumstat(raw string) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		out = append(out, fmt.Sprintf("%s\t%s\t<persisted-%d>", fields[0], fields[1], i+1))
	}
	return strings.Join(out, "\n")
}

func persistableModPowerRoute(id string, a efficiency.RepairAttempt) *modPersistedPowerRoute {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return &modPersistedPowerRoute{
		ID: strings.TrimSpace(id), StrategyID: strings.TrimSpace(a.StrategyID),
		FailureFingerprint: strings.TrimSpace(a.FailureFingerprint), ResolvedFingerprint: strings.TrimSpace(a.ResolvedFingerprint),
		PatchNumstat: sanitizePersistedNumstat(a.PatchNumstat), Regression: a.Regression, Verification: a.Verification,
		EstimatedFlashKZT: a.EstimatedFlashKZT, EstimatedProKZT: a.EstimatedProKZT, Finalization: a.Finalization,
	}
}

func (p *modPersistedPowerRoute) attempt() efficiency.RepairAttempt {
	if p == nil {
		return efficiency.RepairAttempt{}
	}
	return efficiency.RepairAttempt{
		StrategyID: p.StrategyID, FailureFingerprint: p.FailureFingerprint, ResolvedFingerprint: p.ResolvedFingerprint,
		PatchNumstat: p.PatchNumstat, Regression: p.Regression, Verification: p.Verification,
		EstimatedFlashKZT: p.EstimatedFlashKZT, EstimatedProKZT: p.EstimatedProKZT, Finalization: p.Finalization,
	}
}

type modPersistedAppState struct {
	SchemaVersion int                              `json:"schemaVersion"`
	Workspace     string                           `json:"workspace"`
	SavedAtUnixMS int64                            `json:"savedAtUnixMs"`
	Profile       efficiency.ProjectProfile        `json:"profile"`
	ToolDecisions map[string]string                `json:"toolDecisions,omitempty"`
	ApprovalMode  string                           `json:"approvalMode,omitempty"`
	Budget        efficiency.BudgetPersistentState `json:"budget"`
	Execution     efficiency.ExecutionConfig       `json:"execution"`
	Orchestrator  *modAutoConfig                   `json:"orchestrator,omitempty"`
	PendingPower  *modPersistedPowerRoute          `json:"pendingPower,omitempty"`
}

func canonicalModWorkspace(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	return filepath.Clean(root)
}

func modAppStatePath(root string) string {
	base := strings.TrimSpace(config.ReasonixHomeDir())
	if base == "" {
		return ""
	}
	root = canonicalModWorkspace(root)
	sum := sha256.Sum256([]byte(root))
	key := hex.EncodeToString(sum[:12])
	return filepath.Join(base, "balance", "apk-state", key+".json")
}

func (s *Server) modPersistenceStatus() map[string]any {
	s.modPersistMu.Lock()
	defer s.modPersistMu.Unlock()
	return map[string]any{
		"enabled":                    s.modPersistPath != "",
		"path":                       s.modPersistPath,
		"lastError":                  s.modPersistErr,
		"schemaVersion":              modAppStateSchema,
		"preservesBudgetSpend":       true,
		"preservesPendingPowerRoute": true,
	}
}

func (s *Server) captureModAppState() modPersistedAppState {
	auto := s.modAuto.Snapshot().Config
	var pending *modPersistedPowerRoute
	if id, attempt, ok := s.modPowerTurn.PendingRecord(); ok {
		pending = persistableModPowerRoute(id, attempt)
	}
	return modPersistedAppState{
		SchemaVersion: modAppStateSchema,
		Workspace:     canonicalModWorkspace(s.modWorkspacePath()),
		SavedAtUnixMS: time.Now().UnixMilli(),
		Profile:       s.modProfile.Snapshot(),
		ToolDecisions: s.modToolDecisionsSnapshot(),
		ApprovalMode:  s.ctl().ToolApprovalMode(),
		Budget:        s.modGov.PersistentState(),
		Execution:     s.modExec.Snapshot().Config,
		Orchestrator:  &auto,
		PendingPower:  pending,
	}
}

func writeModStateAtomic(path string, state modPersistedAppState) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".apk-state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Server) persistModAppState() error {
	state := s.captureModAppState()
	s.modPersistMu.Lock()
	path := s.modPersistPath
	s.modPersistMu.Unlock()
	err := writeModStateAtomic(path, state)
	s.modPersistMu.Lock()
	if err != nil {
		s.modPersistErr = err.Error()
	} else {
		s.modPersistErr = ""
	}
	s.modPersistMu.Unlock()
	return err
}

func (s *Server) persistModAppStateBestEffort() {
	if err := s.persistModAppState(); err != nil && s.modHub != nil {
		s.modHub.Emit("app.persistence.failed", map[string]any{"reason": err.Error()})
	}
}

func (s *Server) loadModAppState() {
	// No explicit workspace means there is no stable project identity. This is
	// common in unit tests and utility controllers; do not persist process-CWD
	// policy under one shared key in that case.
	if strings.TrimSpace(s.ctl().WorkspaceRoot()) == "" {
		s.modPersistMu.Lock()
		s.modPersistPath = ""
		s.modPersistErr = ""
		s.modPersistMu.Unlock()
		return
	}
	path := modAppStatePath(s.modWorkspacePath())
	s.modPersistMu.Lock()
	s.modPersistPath = path
	s.modPersistErr = ""
	s.modPersistMu.Unlock()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.setModPersistenceError(err)
		return
	}
	if len(data) > 2<<20 {
		s.setModPersistenceError(fmt.Errorf("persisted APK state is too large"))
		return
	}
	var st modPersistedAppState
	if err := json.Unmarshal(data, &st); err != nil {
		s.setModPersistenceError(fmt.Errorf("decode persisted APK state: %w", err))
		return
	}
	if st.SchemaVersion != modAppStateSchema {
		s.setModPersistenceError(fmt.Errorf("unsupported persisted APK state schema %d", st.SchemaVersion))
		return
	}
	if canonicalModWorkspace(st.Workspace) != canonicalModWorkspace(s.modWorkspacePath()) {
		s.setModPersistenceError(fmt.Errorf("persisted APK state belongs to another workspace"))
		return
	}

	for _, pack := range st.Profile.ToolPacks {
		if !s.knownModToolPack(pack) {
			s.setModPersistenceError(fmt.Errorf("persisted state references unknown tool pack %q", pack))
			return
		}
	}
	if _, err := s.modProfile.Configure(st.Profile); err != nil {
		s.setModPersistenceError(err)
		return
	}
	decisions, err := s.canonicalModToolDecisions(st.ToolDecisions)
	if err != nil {
		s.setModPersistenceError(err)
		return
	}
	s.setModToolDecisions(decisions)
	if mode := strings.ToLower(strings.TrimSpace(st.ApprovalMode)); mode == "ask" || mode == "auto" || mode == "yolo" {
		s.ctl().SetToolApprovalMode(mode)
	}
	if _, err := s.modGov.RestorePersistentState(st.Budget); err != nil {
		s.setModPersistenceError(err)
		return
	}
	if err := efficiency.ValidateExecutionConfig(st.Execution); err != nil {
		s.setModPersistenceError(err)
		return
	}
	if st.Execution.Enabled {
		refs := []string{st.Execution.FlashPrimaryRef, st.Execution.FlashAlternativeRef, st.Execution.ProRef, st.Execution.FlashRepairRef}
		for _, ref := range refs {
			if strings.TrimSpace(ref) != "" && !s.modExecutionRefKnown(ref) {
				s.setModPersistenceError(fmt.Errorf("persisted execution model ref is unavailable: %s", ref))
				return
			}
		}
	}
	execSnap, err := s.modExec.Configure(st.Execution, currentModelRef(s.ctl()))
	if err != nil {
		s.setModPersistenceError(err)
		return
	}
	s.setModExecutionPolicyMode(execSnap.Mode)
	if st.Orchestrator != nil {
		if _, err := s.modAuto.Configure(*st.Orchestrator); err != nil {
			s.setModPersistenceError(err)
			return
		}
	}
	if st.PendingPower != nil && strings.TrimSpace(st.PendingPower.ID) != "" {
		s.modPowerTurn.RestorePending(st.PendingPower.ID, st.PendingPower.attempt())
		s.modAuto.noteBlocked("restored durable pending route after backend restart; explicit resume required", true)
	}
	if err := s.applyModToolDecisionsToCurrentController(); err != nil {
		s.setModPersistenceError(err)
		return
	}
	_ = s.applyModTaskCostBudget()
	s.modHub.Emit("app.persistence.restored", map[string]any{"workspace": s.modWorkspacePath()})
}

func (s *Server) setModPersistenceError(err error) {
	if err == nil {
		return
	}
	s.modPersistMu.Lock()
	s.modPersistErr = err.Error()
	s.modPersistMu.Unlock()
}

type modAppApplyRequest struct {
	Profile          *efficiency.ProjectProfile  `json:"profile,omitempty"`
	Budget           *efficiency.BudgetConfig    `json:"budget,omitempty"`
	ResetBudgetSpend bool                        `json:"resetBudgetSpend,omitempty"`
	Execution        *efficiency.ExecutionConfig `json:"execution,omitempty"`
	Orchestrator     *modAutoConfig              `json:"orchestrator,omitempty"`
	ToolDecisions    *map[string]string          `json:"toolDecisions,omitempty"`
	ApprovalMode     *string                     `json:"approvalMode,omitempty"`
}

func (s *Server) validateModAppApply(req modAppApplyRequest) (map[string]string, error) {
	var canonical map[string]string
	if req.ResetBudgetSpend && req.Budget == nil {
		return nil, fmt.Errorf("resetBudgetSpend requires budget")
	}
	if req.Profile != nil {
		if err := efficiency.ValidateProjectProfile(*req.Profile); err != nil {
			return nil, err
		}
		for _, pack := range req.Profile.ToolPacks {
			if !s.knownModToolPack(pack) {
				return nil, fmt.Errorf("unknown tool pack: %s", strings.TrimSpace(pack))
			}
		}
	}
	if req.Budget != nil {
		if err := efficiency.ValidateConfig(*req.Budget); err != nil {
			return nil, err
		}
	}
	if req.Orchestrator != nil {
		if err := validateModAutoConfig(*req.Orchestrator); err != nil {
			return nil, err
		}
	}
	if req.Execution != nil {
		if err := efficiency.ValidateExecutionConfig(*req.Execution); err != nil {
			return nil, err
		}
		if req.Execution.Enabled {
			for _, ref := range []string{req.Execution.FlashPrimaryRef, req.Execution.FlashAlternativeRef, req.Execution.ProRef, req.Execution.FlashRepairRef} {
				if strings.TrimSpace(ref) != "" && !s.modExecutionRefKnown(ref) {
					return nil, fmt.Errorf("unknown model ref: %s", strings.TrimSpace(ref))
				}
			}
		}
	}
	if req.ToolDecisions != nil {
		var err error
		canonical, err = s.canonicalModToolDecisions(*req.ToolDecisions)
		if err != nil {
			return nil, err
		}
	}
	if req.ApprovalMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*req.ApprovalMode))
		if mode != "ask" && mode != "auto" && mode != "yolo" {
			return nil, fmt.Errorf("approvalMode must be ask, auto, or yolo")
		}
	}
	return canonical, nil
}

func (s *Server) modAppApply(w http.ResponseWriter, r *http.Request) {
	if s.modAuto != nil && s.modAuto.BlocksSubmit() {
		http.Error(w, "cannot change APK control state during automatic continuation", http.StatusConflict)
		return
	}
	if s.ctl().Running() {
		http.Error(w, "cannot change APK control state while a turn is running", http.StatusConflict)
		return
	}
	var req modAppApplyRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid app config: "+err.Error(), http.StatusBadRequest)
		return
	}
	canonical, err := s.validateModAppApply(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	beforeProfile := s.modProfile.Snapshot()
	beforeTools := s.modToolDecisionsSnapshot()
	beforeMode := s.ctl().ToolApprovalMode()
	beforeBudget := s.modGov.PersistentState()
	beforeExec := s.modExec.Snapshot()
	beforeAuto := s.modAuto.Snapshot().Config
	rollback := func() {
		_, _ = s.modProfile.Configure(beforeProfile)
		s.setModToolDecisions(beforeTools)
		s.ctl().SetToolApprovalMode(beforeMode)
		_, _ = s.modGov.RestorePersistentState(beforeBudget)
		_, _ = s.modExec.Configure(beforeExec.Config, currentModelRef(s.ctl()))
		_, _ = s.modAuto.Configure(beforeAuto)
		_ = s.applyModToolDecisionsToCurrentController()
		_ = s.applyModTaskCostBudget()
	}

	if req.ToolDecisions != nil {
		s.setModToolDecisions(canonical)
	}
	if req.Profile != nil {
		if _, err := s.modProfile.Configure(*req.Profile); err != nil {
			rollback()
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	if err := s.applyModToolDecisionsToCurrentController(); err != nil {
		rollback()
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if req.ApprovalMode != nil {
		s.ctl().SetToolApprovalMode(strings.ToLower(strings.TrimSpace(*req.ApprovalMode)))
	}
	if req.Budget != nil {
		if req.ResetBudgetSpend {
			s.modGov.Configure(*req.Budget)
		} else {
			preserved := beforeBudget
			preserved.Config = *req.Budget
			if _, err := s.modGov.RestorePersistentState(preserved); err != nil {
				rollback()
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
		}
		_ = s.applyModTaskCostBudget()
	}
	if req.Execution != nil {
		if _, err := s.modExec.Configure(*req.Execution, currentModelRef(s.ctl())); err != nil {
			rollback()
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	if req.Orchestrator != nil {
		if _, err := s.modAuto.Configure(*req.Orchestrator); err != nil {
			rollback()
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	if err := s.persistModAppState(); err != nil {
		rollback()
		http.Error(w, "persist app config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.modHub.Emit("app.config.applied", map[string]any{"workspace": s.modWorkspacePath()})
	s.modAppBootstrap(w, r)
}

func (s *Server) modAppBootstrap(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"protocolVersion": modAPKProtocolVersion,
		"modVersion":      balanceModVersion,
		"contract":        s.modContractSummary(),
		"workspace":       s.modWorkspacePath(),
		"running":         s.ctl().Running(),
		"modelRef":        currentModelRef(s.ctl()),
		"profile":         s.modProfile.Snapshot(),
		"budget":          s.modGov.Snapshot(),
		"execution":       s.modExec.Snapshot(),
		"power":           s.modPower.Snapshot(),
		"powerTurn":       s.modPowerTurn.Snapshot(),
		"orchestrator":    s.modAuto.Snapshot(),
		"quality":         s.modQuality.Snapshot(),
		"recovery":        s.modRecoverySnapshot(),
		"resources":       efficiency.ReadResources(s.modWorkspacePath()),
		"environment":     s.modEnvironmentSnapshot(r.Context(), false),
		"tools":           s.modToolViews(),
		"skills":          s.modSkillViews(),
		"persistence":     s.modPersistenceStatus(),
		"projectManager":  map[string]any{"registryScope": "reasonix-home", "budgetScope": "workspace", "switchMethod": "supervisor-restart"},
		"taskQueue":       s.modQueueView(),
		"transport":       map[string]any{"http": true, "sse": true, "hiddenReasoningExported": false},
		"safety":          map[string]any{"proDiagnosisReadOnly": true, "pendingRouteDurable": true, "continuationIdempotent": true, "restartRequiresExplicitResume": true},
		"endpoints": map[string]string{
			"bootstrap": "/mod/app/bootstrap", "contract": "/mod/app/contract", "apply": "/mod/app/apply",
			"startTask": "/mod/app/task/start", "stopTask": "/mod/app/task/stop",
			"events": "/mod/events", "history": "/mod/live/history",
			"approve": "/approve", "workspaceValidate": "/mod/workspace/validate",
			"projects": "/mod/projects", "registerProject": "/mod/projects/register",
			"openProject": "/mod/projects/open", "removeProject": "/mod/projects/remove",
			"tasks": "/mod/tasks", "renameTask": "/mod/tasks/rename",
			"queue": "/mod/queue", "enqueueTask": "/mod/queue/items",
			"pauseQueue": "/mod/queue/pause", "resumeQueue": "/mod/queue/resume",
			"queueRecovery": "/mod/queue/recovery",
			"orchestrator":  "/mod/orchestrator", "orchestratorConfig": "/mod/orchestrator/config",
			"orchestratorStop": "/mod/orchestrator/stop", "orchestratorResume": "/mod/orchestrator/resume",
			"newTask": "/new", "resumeTask": "/resume", "deleteTask": "/delete-session",
		},
		"workspaceSwitch": map[string]any{
			"live":   false,
			"method": "supervisor-restart",
			"reason": "one Reasonix controller/session stays bound to one process workspace",
		},
	})
}

func (s *Server) modAppPersistenceGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.modPersistenceStatus())
}

func (s *Server) modAppPersistenceSave(w http.ResponseWriter, _ *http.Request) {
	if s.ctl().Running() {
		http.Error(w, "cannot force-save app policy while a turn is running", http.StatusConflict)
		return
	}
	if err := s.persistModAppState(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.modHub.Emit("app.persistence.saved", map[string]any{"workspace": s.modWorkspacePath()})
	writeJSON(w, s.modPersistenceStatus())
}
