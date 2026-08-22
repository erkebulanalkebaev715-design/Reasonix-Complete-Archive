package sessioninbox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const completionReceiptDomain = "reasonix-sessioninbox-completion-v1"

// CommitCompletion atomically marks the whole active set as durably completed.
// It must be called only after the transcript/activity snapshot is durable and
// before AckDequeue. If any ID is missing or is not in an admitted state, no
// item is marked: partial completion receipts are forbidden.
func (s *Store) CommitCompletion(ids []string) error {
	if s == nil {
		return ErrClosed
	}
	wanted := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return ErrNotFound
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		wanted = append(wanted, id)
	}
	if len(wanted) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.beginDiskTransactionLocked()
	if err != nil {
		return err
	}
	defer release()
	if err := s.mutableLocked(); err != nil {
		return err
	}

	// Validate the complete active set against the current transaction before
	// mutating the clone. This is the all-or-nothing boundary.
	for _, id := range wanted {
		meta, ok := s.man.item(id)
		if !ok {
			return ErrNotFound
		}
		if !completionEligible(meta.State) {
			return ErrInvalidState
		}
	}

	next := s.man.clone()
	now := time.Now().UTC()
	for _, id := range wanted {
		i := next.indexOf(id)
		if i < 0 {
			return ErrNotFound
		}
		if next.Items[i].CompletionReceipt == "" {
			next.Items[i].CompletionReceipt = completionReceipt(next.Items[i])
			next.Items[i].CompletionCommittedAt = now
			next.Items[i].UpdatedAt = now
		}
	}
	if err := s.commitManifestLocked(next); err != nil {
		return err
	}
	s.notifyLocked(s.snapshotLocked())
	return nil
}

func completionEligible(state InboxState) bool {
	switch state {
	case StateRunning, StateSteerAccepted, StateSteerConsumed:
		return true
	default:
		return false
	}
}

func completionReceipt(item InboxItemMeta) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\n%s\n%d\n%s\n%s\n", completionReceiptDomain, item.ID, item.Revision, item.Checksum, item.RunID)
	return hex.EncodeToString(h.Sum(nil))
}

func hasDurableCompletion(item InboxItemMeta) bool {
	return item.CompletionReceipt != "" && !item.CompletionCommittedAt.IsZero() && validSHA256(item.CompletionReceipt)
}

// finalizeCommittedOrphans removes completed-but-unacknowledged active items
// from a manifest clone. Existing client idempotency keys receive the same
// acknowledged receipt AckDequeue would have created, so retries remain safe.
// It returns removed blob stems for post-commit cleanup.
func finalizeCommittedOrphans(m *manifest, ownedBy func(string) bool, now time.Time) []string {
	if m == nil {
		return nil
	}
	ids := make([]string, 0)
	for _, item := range m.Items {
		if ownedBy != nil && ownedBy(item.ID) {
			continue
		}
		if hasDurableCompletion(item) {
			ids = append(ids, item.ID)
		}
	}
	blobs := make([]string, 0, len(ids))
	for _, id := range ids {
		_, ok := m.item(id)
		if !ok {
			continue
		}
		keys := m.idempotencyKeysFor(id)
		m.rememberReceipt(keys, id, Disposition("acknowledged"), now)
		removed, ok := m.removeItem(id)
		if ok {
			blobs = append(blobs, blobNameFor(removed))
		}
	}
	clearPauseIfEmpty(m)
	return blobs
}
