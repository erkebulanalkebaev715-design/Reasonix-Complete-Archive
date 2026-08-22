package serve

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"reasonix/internal/efficiency"
	"reasonix/internal/event"
	"reasonix/internal/secrets"
)

func modBoundedRedacted(s string, max int) string {
	s = secrets.RedactCredentials(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n…[truncated]"
}

func (s *Server) modProjectDetailsEnabled() bool {
	return s.modProfile.Snapshot().LiveDetail == efficiency.LiveDetailProject
}

// observeLiveCoreEvent translates Reasonix's typed event stream into the
// stable APK protocol. It intentionally never mirrors event.Reasoning or the
// Message.Reasoning field: the APK gets visible chat text, actions, diffs,
// command results and verification state, not hidden chain-of-thought.
func (s *Server) observeLiveCoreEvent(e event.Event) {
	details := s.modProjectDetailsEnabled()
	switch e.Kind {
	case event.TurnStarted:
		s.modHub.Emit("live.turn.started", map[string]any{"modelRef": e.ModelRef})
	case event.Text:
		if e.Text != "" {
			s.modHub.EmitTransient("live.chat.delta", map[string]any{"text": modBoundedRedacted(e.Text, 16<<10), "source": e.Source})
		}
	case event.Message:
		if e.Text != "" {
			s.modHub.Emit("live.chat.message", map[string]any{"text": modBoundedRedacted(e.Text, 64<<10), "source": e.Source})
		}
	case event.Phase:
		s.modHub.Emit("live.phase", map[string]any{"label": modBoundedRedacted(e.Text, 512), "source": e.Source})
	case event.TurnPhase:
		s.modHub.Emit("live.phase", map[string]any{"phase": string(e.PhaseName)})
	case event.ToolDispatch:
		payload := map[string]any{
			"id": e.Tool.ID, "name": e.Tool.Name, "resolvedName": e.Tool.ResolvedName,
			"capabilityId": e.Tool.CapabilityID, "readOnly": e.Tool.ReadOnly,
			"partial": e.Tool.Partial, "parentId": e.Tool.ParentID,
			"added": e.Tool.Added, "removed": e.Tool.Removed,
		}
		if details && !e.Tool.Partial {
			payload["argsPreview"] = modBoundedRedacted(e.Tool.Args, 4<<10)
			if e.Tool.Diff != "" {
				payload["diff"] = modBoundedRedacted(e.Tool.Diff, 12<<10)
			}
		}
		s.modHub.Emit("live.tool.started", payload)
	case event.ToolProgress:
		if details && e.Tool.Output != "" {
			s.modHub.EmitTransient("live.tool.progress", map[string]any{"id": e.Tool.ID, "name": e.Tool.Name, "outputPreview": modBoundedRedacted(e.Tool.Output, 2<<10)})
		}
	case event.ToolResult, event.ToolResultPreview:
		payload := map[string]any{
			"id": e.Tool.ID, "name": e.Tool.Name, "resolvedName": e.Tool.ResolvedName,
			"ok": e.Tool.Err == "", "durationMs": e.Tool.DurationMs, "truncated": e.Tool.Truncated,
		}
		if e.Tool.Err != "" {
			payload["error"] = modBoundedRedacted(e.Tool.Err, 4<<10)
		}
		if details && e.Tool.Output != "" {
			payload["outputPreview"] = modBoundedRedacted(e.Tool.Output, 8<<10)
		}
		if e.Tool.Execution != nil {
			payload["execution"] = map[string]any{
				"kind": e.Tool.Execution.Kind, "state": e.Tool.Execution.State,
				"failurePhase": e.Tool.Execution.FailurePhase, "exitCode": e.Tool.Execution.ExitCode,
				"verification": e.Tool.Execution.Verification, "durationMs": e.Tool.Execution.DurationMs,
			}
		}
		s.modHub.Emit("live.tool.finished", payload)
	case event.ApprovalRequest:
		payload := map[string]any{"id": e.Approval.ID, "tool": e.Approval.Tool}
		if details && e.Approval.Subject != "" {
			payload["subject"] = modBoundedRedacted(e.Approval.Subject, 2<<10)
		}
		s.modHub.Emit("live.approval.requested", payload)
	case event.WorkspaceChanged:
		if e.Workspace == nil {
			return
		}
		payload := map[string]any{"changeCount": len(e.Workspace.Changes), "allPaths": e.Workspace.AllPaths, "source": e.Workspace.Source}
		if details {
			const maxChanges = 128
			count := len(e.Workspace.Changes)
			if count > maxChanges {
				count = maxChanges
				payload["changesTruncated"] = true
			}
			changes := make([]map[string]string, 0, count)
			for _, c := range e.Workspace.Changes[:count] {
				changes = append(changes, map[string]string{
					"path": modBoundedRedacted(c.Path, 1024), "oldPath": modBoundedRedacted(c.OldPath, 1024), "op": c.Op,
				})
			}
			payload["changes"] = changes
		}
		s.modHub.Emit("live.workspace.changed", payload)
	case event.Retrying:
		s.modHub.Emit("live.provider.retry", map[string]any{"attempt": e.RetryAttempt, "max": e.RetryMax, "scope": string(e.RetryScope)})
	case event.CompletionSummary:
		if e.Completion != nil {
			s.modHub.Emit("live.verification.summary", e.Completion)
		}
	case event.TurnDone:
		payload := map[string]any{"cancelled": e.Cancelled, "outcome": e.Outcome}
		if e.Err != nil {
			payload["error"] = secrets.RedactError(e.Err)
		}
		if e.Readiness != nil {
			payload["readiness"] = e.Readiness
		}
		s.modHub.Emit("live.turn.done", payload)
	}
}

func (s *Server) modLiveHistory(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 512 {
			http.Error(w, "limit must be 1..512", http.StatusBadRequest)
			return
		}
		limit = n
	}
	raw := s.modHub.History(512)
	out := make([]modEvent, 0, len(raw))
	for _, item := range raw {
		var e modEvent
		if json.Unmarshal(item, &e) != nil || !strings.HasPrefix(e.Type, "live.") {
			continue
		}
		out = append(out, e)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	writeJSON(w, map[string]any{"events": out, "count": len(out)})
}
