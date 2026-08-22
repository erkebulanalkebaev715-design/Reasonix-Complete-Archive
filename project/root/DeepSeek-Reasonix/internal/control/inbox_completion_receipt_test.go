package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

func TestV017ControllerCommitsCompletionBeforeAck(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, Sink: event.Discard})
	st, err := c.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := c.EnqueueInbox(InboxRequest{Submit: "v0.17 crash-boundary"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(rec.ItemID, sessioninbox.StateRunning, ""); err != nil {
		t.Fatal(err)
	}
	c.inbox.mu.Lock()
	c.inbox.clearActive()
	c.inbox.trackActive(rec.ItemID)
	c.inbox.beforeCompletionAck = func() {
		meta, _, readErr := st.ReadItem(rec.ItemID)
		if readErr != nil {
			panic(readErr)
		}
		if meta.CompletionReceipt == "" || meta.CompletionCommittedAt.IsZero() {
			panic("v0.17 completion receipt missing before ack")
		}
		panic("v0.17 simulated crash after completion commit")
	}
	c.inbox.mu.Unlock()

	crashed := false
	func() {
		defer func() {
			if recover() != nil {
				crashed = true
			}
		}()
		c.onInboxTurnDone()
	}()
	if !crashed {
		t.Fatal("simulated crash hook did not run")
	}

	// Simulate restart ownership loss. The committed turn must disappear from
	// the inbox rather than becoming uncertain/retryable.
	recovered, err := st.RecoverOrphanedInFlight(nil)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 0 {
		t.Fatalf("post-receipt crash became uncertain: %d", recovered)
	}
	if snap := st.Snapshot(); snap.Paused || len(snap.Items) != 0 {
		t.Fatalf("post-receipt crash not finalized: %+v", snap)
	}
}
