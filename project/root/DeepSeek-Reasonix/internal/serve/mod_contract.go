package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
)

const modAPKContractRevision = 1

type modContractEndpoint struct {
	Name        string `json:"name"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	JSONBody    bool   `json:"jsonBody,omitempty"`
	Description string `json:"description,omitempty"`
}

type modAPKContract struct {
	ProtocolVersion  string                `json:"protocolVersion"`
	ContractRevision int                   `json:"contractRevision"`
	Digest           string                `json:"digest"`
	Compatibility    map[string]any        `json:"compatibility"`
	RequestRules     map[string]any        `json:"requestRules"`
	Guarantees       map[string]any        `json:"guarantees"`
	Endpoints        []modContractEndpoint `json:"endpoints"`
	Events           []string              `json:"events"`
}

// modAPKContractEndpoints is the frozen v1 Android-facing surface. Additive
// endpoints are allowed under balance-apk-v1, but removing/renaming an entry or
// changing its method/meaning requires a protocol-major bump.
var modAPKContractEndpoints = []modContractEndpoint{
	{"bootstrap", "GET", "/mod/app/bootstrap", false, "single startup snapshot"},
	{"contract", "GET", "/mod/app/contract", false, "versioned APK protocol manifest"},
	{"apply", "POST", "/mod/app/apply", true, "atomic idle policy apply"},
	{"startTask", "POST", "/mod/app/task/start", true, "native submit alias"},
	{"stopTask", "POST", "/mod/app/task/stop", true, "native cancel alias"},
	{"persistence", "GET", "/mod/app/persistence", false, "workspace policy persistence status"},
	{"savePersistence", "POST", "/mod/app/persistence/save", true, "force idle policy save"},
	{"status", "GET", "/mod/status", false, "aggregate mod state"},
	{"budget", "GET", "/mod/budget", false, "KZT budget snapshot"},
	{"configureBudget", "POST", "/mod/budget", true, "configure KZT budget"},
	{"resetBudget", "POST", "/mod/budget/reset", true, "reset spend ledger"},
	{"resources", "GET", "/mod/resources", false, "local device/resource snapshot"},
	{"quality", "GET", "/mod/quality", false, "anti-loop/completion telemetry"},
	{"router", "GET", "/mod/router", false, "objective route state"},
	{"resetRouter", "POST", "/mod/router/reset", true, "reset route state"},
	{"cycle", "GET", "/mod/cycle", false, "repair-cycle state"},
	{"resetCycle", "POST", "/mod/cycle/reset", true, "reset repair cycle"},
	{"execution", "GET", "/mod/execution", false, "execution model slots/state"},
	{"configureExecution", "POST", "/mod/execution/config", true, "configure model slots"},
	{"resetExecution", "POST", "/mod/execution/reset", true, "reset execution routing"},
	{"power", "GET", "/mod/power", false, "unified economy/power engine state"},
	{"resetPower", "POST", "/mod/power/reset", true, "reset unified power state"},
	{"applyPendingPower", "POST", "/mod/power/apply-pending", true, "explicit idle-boundary route apply"},
	{"orchestrator", "GET", "/mod/orchestrator", false, "automatic continuation state"},
	{"configureOrchestrator", "POST", "/mod/orchestrator/config", true, "configure bounded continuation"},
	{"stopOrchestrator", "POST", "/mod/orchestrator/stop", true, "stop automatic continuation"},
	{"resumeOrchestrator", "POST", "/mod/orchestrator/resume", true, "explicitly resume after restart/stop"},
	{"agent", "GET", "/mod/agent", false, "agent control snapshot"},
	{"reloadAgent", "POST", "/mod/agent/reload", true, "same-model native controller rebuild"},
	{"capabilities", "GET", "/mod/capabilities", false, "native tool registry projection"},
	{"environment", "GET", "/mod/environment", false, "native Reasonix environment projection"},
	{"projectProfile", "GET", "/mod/project", false, "active project profile"},
	{"setProjectProfile", "POST", "/mod/project", true, "set chat/agent mode and tool packs"},
	{"agentTools", "GET", "/mod/agent/tools", false, "per-tool permission state"},
	{"setAgentTools", "POST", "/mod/agent/tools", true, "set allow/ask/deny overrides"},
	{"skills", "GET", "/mod/agent/skills", false, "skill state"},
	{"setSkills", "POST", "/mod/agent/skills", true, "enable/disable skills"},
	{"instructions", "GET", "/mod/instructions", false, "recognized standing instruction documents"},
	{"setInstructions", "POST", "/mod/instructions", true, "edit recognized instruction document"},
	{"workspace", "GET", "/mod/workspace", false, "active workspace"},
	{"validateWorkspace", "POST", "/mod/workspace/validate", true, "validate supervisor workspace target"},
	{"workspaceFiles", "GET", "/mod/workspace/files", false, "confined workspace browser"},
	{"workspaceFile", "GET", "/mod/workspace/file", false, "bounded confined file preview"},
	{"recovery", "GET", "/mod/recovery", false, "checkpoint/recovery state"},
	{"rollback", "POST", "/mod/recovery/rollback-last", true, "native fail-closed code rewind"},
	{"events", "GET", "/mod/events", false, "typed SSE stream"},
	{"liveHistory", "GET", "/mod/live/history", false, "bounded retained live event history"},
	{"projects", "GET", "/mod/projects", false, "project registry"},
	{"registerProject", "POST", "/mod/projects/register", true, "register existing workspace"},
	{"removeProject", "POST", "/mod/projects/remove", true, "unregister without deleting files"},
	{"openProject", "POST", "/mod/projects/open", true, "request supervisor restart handoff"},
	{"tasks", "GET", "/mod/tasks", false, "native session-backed task catalog"},
	{"renameTask", "POST", "/mod/tasks/rename", true, "rename native session"},
	{"queue", "GET", "/mod/queue", false, "native durable inbox projection"},
	{"enqueueTask", "POST", "/mod/queue/items", true, "enqueue durable task"},
	{"updateQueuedTask", "PATCH", "/mod/queue/items/{id}", true, "update queued task"},
	{"deleteQueuedTask", "DELETE", "/mod/queue/items/{id}", false, "delete queued task"},
	{"moveQueuedTask", "POST", "/mod/queue/move", true, "reorder queue"},
	{"pauseQueue", "POST", "/mod/queue/pause", true, "pause native inbox"},
	{"resumeQueue", "POST", "/mod/queue/resume", true, "resume native inbox"},
	{"retryQueuedTask", "POST", "/mod/queue/items/{id}/retry", true, "explicit retry"},
	{"queueRecovery", "GET", "/mod/queue/recovery", false, "review uncertain recovered work"},
	{"retryQueueRecovery", "POST", "/mod/queue/recovery/retry", true, "explicit recovered-task retry"},
	{"approve", "POST", "/approve", true, "native interactive tool approval"},
	{"newTask", "POST", "/new", true, "native new session"},
	{"resumeTask", "POST", "/resume", true, "native session resume"},
	{"deleteTask", "POST", "/delete-session", true, "native session delete"},
	{"swarmStart", "POST", "/mod/swarm/start", true, "start a bounded swarm run"},
	{"swarmCancel", "POST", "/mod/swarm/cancel", true, "cancel the active swarm"},
	{"swarmStatus", "GET", "/mod/swarm", false, "active swarm structured state"},
	{"swarmByID", "GET", "/mod/swarm/{id}", false, "persisted swarm state by id"},
	{"swarmHistory", "GET", "/mod/swarm/history", false, "recent persisted swarm states"},
}

var modAPKContractEvents = []string{
	"agent.instructions.updated", "agent.reloaded", "agent.skill.updated", "agent.tools.updated",
	"app.config.applied", "app.persistence.failed", "app.persistence.restored", "app.persistence.saved",
	"budget.configured", "budget.exhausted", "budget.pro_limit_reached", "budget.reserve_entered", "budget.reset", "budget.updated",
	"execution.configured", "execution.reset", "execution.updated",
	"live.approval.requested", "live.chat.delta", "live.chat.message", "live.phase", "live.provider.retry",
	"live.tool.finished", "live.tool.progress", "live.tool.started", "live.turn.done", "live.turn.started", "live.verification.summary", "live.workspace.changed",
	"model.escalated", "model.returned_flash", "model.switch.completed",
	"orchestrator.blocked", "orchestrator.configured", "orchestrator.continuation", "orchestrator.finished", "orchestrator.pending", "orchestrator.recovered", "orchestrator.scheduled", "orchestrator.stopped",
	"power.blocked", "power.reset", "power.route.pending", "power.route.persistence_failed", "power.turn.observed", "power.updated",
	"project.open.requested", "project.profile.updated", "project.registered", "project.unregistered",
	"quality.updated",
	"queue.item.admitted", "queue.item.deleted", "queue.item.retried", "queue.item.updated", "queue.paused", "queue.recovery.reviewed", "queue.reordered", "queue.resumed", "queue.updated",
	"repair.completed", "repair.reset", "repair.updated",
	"rollback.completed", "rollback.failed", "router.decision", "router.reset", "task.renamed",
	"swarm.started", "swarm.task.created", "swarm.task.assigned", "swarm.agent.started",
	"swarm.agent.tool_dispatch", "swarm.agent.tool_result",
	"swarm.agent.completed", "swarm.agent.failed", "swarm.task.completed", "swarm.task.failed",
	"swarm.merge.started", "swarm.merge.completed", "swarm.verification.started",
	"swarm.verification.completed", "swarm.completed", "swarm.failed", "swarm.cancelled",
}

type modAPKContractCanonical struct {
	ProtocolVersion  string                `json:"protocolVersion"`
	ContractRevision int                   `json:"contractRevision"`
	Compatibility    map[string]any        `json:"compatibility"`
	RequestRules     map[string]any        `json:"requestRules"`
	Guarantees       map[string]any        `json:"guarantees"`
	Endpoints        []modContractEndpoint `json:"endpoints"`
	Events           []string              `json:"events"`
}

func canonicalModAPKContract() modAPKContractCanonical {
	endpoints := append([]modContractEndpoint(nil), modAPKContractEndpoints...)
	events := append([]string(nil), modAPKContractEvents...)
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		if endpoints[i].Method != endpoints[j].Method {
			return endpoints[i].Method < endpoints[j].Method
		}
		return endpoints[i].Name < endpoints[j].Name
	})
	sort.Strings(events)
	return modAPKContractCanonical{
		ProtocolVersion:  modAPKProtocolVersion,
		ContractRevision: modAPKContractRevision,
		Compatibility: map[string]any{
			"major":                  1,
			"additiveChangesAllowed": true,
			"breakingChangesRequireProtocolMajorBump": true,
			"unknownJSONFieldsClientMustIgnore":       true,
		},
		RequestRules: map[string]any{
			"mutatingRequestsContentType":                 "application/json",
			"unknownRequestFieldsRejected":                true,
			"directHTTPSubmitBlockedDuringAutoTransition": true,
			"workspaceSwitch":                             "supervisor-restart",
		},
		Guarantees: map[string]any{
			"singleAgentController":          true,
			"hiddenReasoningExported":        false,
			"proDiagnosisReadOnly":           true,
			"pendingRouteDurable":            true,
			"continuationIdempotent":         true,
			"restartRequiresExplicitResume":  true,
			"budgetEnforcedInBackend":        true,
			"apkNeverAuthoritativeForSafety": true,
		},
		Endpoints: endpoints,
		Events:    events,
	}
}

func modAPKContractDigest() string {
	raw, _ := json.Marshal(canonicalModAPKContract())
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func modAPKContractPayload() modAPKContract {
	c := canonicalModAPKContract()
	return modAPKContract{
		ProtocolVersion:  c.ProtocolVersion,
		ContractRevision: c.ContractRevision,
		Digest:           modAPKContractDigest(),
		Compatibility:    c.Compatibility,
		RequestRules:     c.RequestRules,
		Guarantees:       c.Guarantees,
		Endpoints:        c.Endpoints,
		Events:           c.Events,
	}
}

func (s *Server) modContractSummary() map[string]any {
	return map[string]any{
		"protocolVersion":  modAPKProtocolVersion,
		"contractRevision": modAPKContractRevision,
		"digest":           modAPKContractDigest(),
		"endpoint":         "/mod/app/contract",
	}
}

func (s *Server) modAppContract(w http.ResponseWriter, r *http.Request) {
	writeJSONCached(w, r, modAPKContractPayload())
}
