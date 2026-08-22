package sessioninbox

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestV017CommitCompletionIsAtomic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "s.jsonl"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	a, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimItem(a.ItemID); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimItem(b.ItemID); err != nil {
		t.Fatal(err)
	}

	if err := s.CommitCompletion([]string{a.ItemID, "missing-v017-item"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("atomic invalid set error = %v, want ErrNotFound", err)
	}
	meta, _, err := s.ReadItem(a.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.CompletionReceipt != "" || !meta.CompletionCommittedAt.IsZero() {
		t.Fatalf("partial completion leaked after rejected set: %+v", meta)
	}

	if err := s.CommitCompletion([]string{a.ItemID, b.ItemID}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a.ItemID, b.ItemID} {
		meta, _, err := s.ReadItem(id)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDurableCompletion(meta) {
			t.Fatalf("item %s missing durable completion: %+v", id, meta)
		}
	}
}

func TestV017CrashBeforeCompletionReceiptBecomesUncertain(t *testing.T) {
	session := filepath.Join(t.TempDir(), "s.jsonl")
	s, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "unsafe to replay blindly"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimItem(rec.ItemID); err != nil {
		t.Fatal(err)
	}
	forceV017ForeignRun(t, s)
	s.Close()

	reopened, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	snap := reopened.Snapshot()
	if !snap.Paused || !snap.Recovered || len(snap.Items) != 1 || snap.Items[0].State != StateUncertain {
		t.Fatalf("pre-receipt crash recovery = %+v", snap)
	}
}

func TestV017CrashAfterCompletionReceiptFinalizesWithoutReplay(t *testing.T) {
	session := filepath.Join(t.TempDir(), "s.jsonl")
	s, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Enqueue(EnqueueRequest{
		Envelope:    PromptEnvelope{SubmitText: "write once"},
		Idempotency: "v017-msg-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimItem(first.ItemID); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitCompletion([]string{first.ItemID}); err != nil {
		t.Fatal(err)
	}
	forceV017ForeignRun(t, s)
	s.Close()

	reopened, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	snap := reopened.Snapshot()
	if snap.Paused || len(snap.Items) != 0 {
		t.Fatalf("post-receipt crash should finalize without pending replay: %+v", snap)
	}

	retry, err := reopened.Enqueue(EnqueueRequest{
		Envelope:    PromptEnvelope{SubmitText: "write once"},
		Idempotency: "v017-msg-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !retry.Idempotent || retry.ItemID != first.ItemID || len(reopened.Snapshot().Items) != 0 {
		t.Fatalf("completed retry = %+v snapshot=%+v", retry, reopened.Snapshot())
	}
}

func TestV017SameProcessRecoveryFinalizesCommittedOrphan(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "s.jsonl"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rec, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "done"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimItem(rec.ItemID); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitCompletion([]string{rec.ItemID}); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.RecoverOrphanedInFlight(nil)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 0 {
		t.Fatalf("committed completion counted uncertain = %d", recovered)
	}
	if snap := s.Snapshot(); snap.Paused || len(snap.Items) != 0 {
		t.Fatalf("committed orphan not finalized: %+v", snap)
	}
}

func forceV017ForeignRun(t *testing.T, s *Store) {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	release, err := s.beginDiskTransactionLocked()
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// commitManifestLocked always stamps next.RunID from s.runID.
	// Temporarily give this Store a foreign run ID so the on-disk manifest
	// really looks like it was left by another crashed process.
	originalRunID := s.runID
	foreignRunID := newRandomID()
	if foreignRunID == originalRunID {
		t.Fatal("test foreign run unexpectedly matches current run")
	}

	s.runID = foreignRunID
	defer func() {
		s.runID = originalRunID
	}()

	next := s.man.clone()
	if err := s.commitManifestLocked(next); err != nil {
		t.Fatal(err)
	}
}

func TestV017CommittedUncertainItemCannotBeRetriedAndRecoveryFinalizesIt(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "s.jsonl"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rec, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "completed"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimItem(rec.ItemID); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitCompletion([]string{rec.ItemID}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetState(rec.ItemID, StateUncertain, "simulated ack failure"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryItem(rec.ItemID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("retry of completed item error = %v, want ErrInvalidState", err)
	}
	if recovered, err := s.RecoverOrphanedInFlight(nil); err != nil || recovered != 0 {
		t.Fatalf("finalize committed uncertain: recovered=%d err=%v", recovered, err)
	}
	if items := s.Snapshot().Items; len(items) != 0 {
		t.Fatalf("committed uncertain item survived recovery: %+v", items)
	}
}
