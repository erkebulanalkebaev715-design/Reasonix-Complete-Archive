package efficiency

import (
	"errors"
	"testing"

	"reasonix/internal/event"
)

func TestQualityMonitorMirrorsHostGuardsAndCompletion(t *testing.T) {
	m := NewQualityMonitor()
	m.Observe(event.Event{Kind: event.TurnStarted})
	m.Observe(event.Event{Kind: event.Notice, Code: event.NoticeCodeLoopGuard})
	m.Observe(event.Event{Kind: event.Notice, Code: event.NoticeCodeProgressGuard})
	m.Observe(event.Event{Kind: event.Notice, Code: event.NoticeCodeEvidenceNudge})
	m.Observe(event.Event{Kind: event.CompletionSummary, Completion: &event.CompletionSummaryInfo{
		Verdict: "continue", ChecksPassed: 2, ChecksFailed: 1, Review: "warned",
	}})

	snap, signals := m.Observe(event.Event{
		Kind: event.TurnDone, Outcome: event.TurnOutcomeFinalReadiness,
		Readiness: &event.FinalReadiness{Attempts: 1, Missing: []string{"verification"}},
	})
	if snap.State != "blocked" || snap.LoopGuardHits != 1 || snap.ProgressGuardHits != 1 || snap.EvidenceNudges != 1 || snap.ReadinessBlocks != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if len(snap.Missing) != 1 || snap.Missing[0] != "verification" {
		t.Fatalf("missing = %#v", snap.Missing)
	}
	found := false
	for _, sig := range signals {
		if sig.Type == "completion.blocked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("signals = %#v, want completion.blocked", signals)
	}
}

func TestQualityMonitorPassAndFailure(t *testing.T) {
	m := NewQualityMonitor()
	snap, _ := m.Observe(event.Event{Kind: event.TurnDone})
	if snap.State != "done" || snap.TurnsPassed != 1 {
		t.Fatalf("pass snapshot = %+v", snap)
	}
	snap, _ = m.Observe(event.Event{Kind: event.TurnDone, Err: errors.New("boom")})
	if snap.State != "failed" || snap.TurnsFailed != 1 {
		t.Fatalf("failure snapshot = %+v", snap)
	}
}
