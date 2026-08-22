package efficiency

import (
	"sync"
	"time"

	"reasonix/internal/event"
)

// QualitySnapshot is content-free Balance Mod state for the APK. It mirrors
// host-owned progress/readiness signals instead of asking the model whether it
// thinks the task is complete.
type QualitySnapshot struct {
	State             string   `json:"state"`
	Phase             string   `json:"phase,omitempty"`
	LoopGuardHits     int      `json:"loopGuardHits"`
	ProgressGuardHits int      `json:"progressGuardHits"`
	EvidenceNudges    int      `json:"evidenceNudges"`
	ReadinessBlocks   int      `json:"readinessBlocks"`
	TurnsPassed       int      `json:"turnsPassed"`
	TurnsFailed       int      `json:"turnsFailed"`
	CompletionVerdict string   `json:"completionVerdict,omitempty"`
	ChecksPassed      int      `json:"checksPassed"`
	ChecksFailed      int      `json:"checksFailed"`
	Review            string   `json:"review,omitempty"`
	Missing           []string `json:"missing,omitempty"`
	UpdatedAtUnixMS   int64    `json:"updatedAtUnixMs"`
}

// QualitySignal is a machine event emitted to the Balance Mod SSE stream.
type QualitySignal struct {
	Type string
	Data any
}

// QualityMonitor consumes Reasonix's existing host-owned guards and completion
// receipts. It does not duplicate those mechanisms or infer truth from model
// prose; the authoritative gate remains Reasonix final-readiness/evidence.
type QualityMonitor struct {
	mu   sync.Mutex
	snap QualitySnapshot
}

func NewQualityMonitor() *QualityMonitor {
	return &QualityMonitor{snap: QualitySnapshot{State: "idle"}}
}

func (m *QualityMonitor) Snapshot() QualitySnapshot {
	if m == nil {
		return QualitySnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneQualitySnapshot(m.snap)
}

func cloneQualitySnapshot(in QualitySnapshot) QualitySnapshot {
	out := in
	out.Missing = append([]string(nil), in.Missing...)
	return out
}

// Observe folds one core event into APK-facing state and returns zero or more
// typed signals. Only content-free event metadata is copied.
func (m *QualityMonitor) Observe(e event.Event) (QualitySnapshot, []QualitySignal) {
	if m == nil {
		return QualitySnapshot{}, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var signals []QualitySignal
	touch := false

	switch e.Kind {
	case event.TurnStarted:
		m.snap.State = "working"
		m.snap.Phase = "working"
		m.snap.CompletionVerdict = ""
		m.snap.ChecksPassed = 0
		m.snap.ChecksFailed = 0
		m.snap.Review = ""
		m.snap.Missing = nil
		signals = append(signals, QualitySignal{Type: "task.turn_started"})
		touch = true

	case event.TurnPhase:
		if e.PhaseName != "" {
			m.snap.Phase = string(e.PhaseName)
			m.snap.State = string(e.PhaseName)
			signals = append(signals, QualitySignal{Type: "phase.changed", Data: map[string]any{"phase": m.snap.Phase}})
			touch = true
		}

	case event.Notice:
		switch e.Code {
		case event.NoticeCodeLoopGuard:
			m.snap.LoopGuardHits++
			m.snap.State = "replanning"
			signals = append(signals, QualitySignal{Type: "loop.detected", Data: map[string]any{"count": m.snap.LoopGuardHits}})
			touch = true
		case event.NoticeCodeProgressGuard:
			m.snap.ProgressGuardHits++
			m.snap.State = "stalled"
			signals = append(signals, QualitySignal{Type: "progress.stalled", Data: map[string]any{"count": m.snap.ProgressGuardHits}})
			touch = true
		case event.NoticeCodeEvidenceNudge:
			m.snap.EvidenceNudges++
			m.snap.State = "needs_evidence"
			signals = append(signals, QualitySignal{Type: "verifier.evidence_required", Data: map[string]any{"count": m.snap.EvidenceNudges}})
			touch = true
		}

	case event.CompletionSummary:
		if e.Completion != nil {
			m.snap.CompletionVerdict = e.Completion.Verdict
			m.snap.ChecksPassed = e.Completion.ChecksPassed
			m.snap.ChecksFailed = e.Completion.ChecksFailed
			m.snap.Review = e.Completion.Review
			data := map[string]any{
				"verdict":      m.snap.CompletionVerdict,
				"checksPassed": m.snap.ChecksPassed,
				"checksFailed": m.snap.ChecksFailed,
				"review":       m.snap.Review,
			}
			signals = append(signals, QualitySignal{Type: "completion.summary", Data: data})
			if m.snap.ChecksFailed > 0 {
				signals = append(signals, QualitySignal{Type: "verifier.failed", Data: data})
			} else if m.snap.ChecksPassed > 0 {
				signals = append(signals, QualitySignal{Type: "verifier.passed", Data: data})
			}
			touch = true
		}

	case event.TurnDone:
		if e.Outcome == event.TurnOutcomeFinalReadiness {
			m.snap.ReadinessBlocks++
			m.snap.State = "blocked"
			m.snap.Missing = nil
			if e.Readiness != nil {
				m.snap.Missing = append([]string(nil), e.Readiness.Missing...)
			}
			signals = append(signals, QualitySignal{Type: "completion.blocked", Data: map[string]any{
				"count":   m.snap.ReadinessBlocks,
				"missing": append([]string(nil), m.snap.Missing...),
			}})
		} else if e.Err != nil {
			m.snap.TurnsFailed++
			m.snap.State = "failed"
			signals = append(signals, QualitySignal{Type: "task.failed", Data: map[string]any{"count": m.snap.TurnsFailed}})
		} else {
			m.snap.TurnsPassed++
			m.snap.State = "done"
			m.snap.Missing = nil
			signals = append(signals, QualitySignal{Type: "completion.allowed", Data: map[string]any{"count": m.snap.TurnsPassed}})
		}
		touch = true
	}

	if touch {
		m.snap.UpdatedAtUnixMS = time.Now().UnixMilli()
	}
	return cloneQualitySnapshot(m.snap), signals
}
