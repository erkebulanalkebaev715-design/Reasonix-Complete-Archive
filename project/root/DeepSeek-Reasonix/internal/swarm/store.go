package swarm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reasonix/internal/fileutil"
)

// Store persists SwarmState under the Reasonix home so a swarm can be observed
// and resumed after a Reasonix/backend restart without re-paying completed
// work. Only structured state is persisted; raw worker transcripts, prompts,
// and hidden reasoning are never written.
type Store struct {
	dir string
}

// NewStore builds a swarm state store rooted at dir. An empty dir makes every
// operation a no-op failure-free path so tests and headless runs can pass a
// nil store.
func NewStore(dir string) *Store {
	return &Store{dir: strings.TrimSpace(dir)}
}

func (s *Store) pathFor(id string) string {
	return filepath.Join(s.dir, sanitizeID(id)+".json")
}

func sanitizeID(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Save persists one swarm state atomically (temp file, 0600, rename).
func (s *Store) Save(state *SwarmState) error {
	if s == nil || s.dir == "" || state == nil || state.ID == "" {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("swarm store: encode %s: %w", state.ID, err)
	}
	if err := fileutil.AtomicWriteFile(s.pathFor(state.ID), b, 0o600); err != nil {
		return fmt.Errorf("swarm store: write %s: %w", state.ID, err)
	}
	return nil
}

// Load returns one swarm state by ID, or nil when absent.
func (s *Store) Load(id string) (*SwarmState, error) {
	if s == nil || s.dir == "" {
		return nil, nil
	}
	b, err := os.ReadFile(s.pathFor(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state SwarmState
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, fmt.Errorf("swarm store: decode %s: %w", id, err)
	}
	if state.ID == "" {
		state.ID = id
	}
	return &state, nil
}

// List returns the persisted swarm states, newest first.
func (s *Store) List() ([]*SwarmState, error) {
	if s == nil || s.dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*SwarmState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		st, err := s.Load(id)
		if err != nil {
			continue
		}
		if st != nil {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Delete removes one swarm state file. Missing state is not an error.
func (s *Store) Delete(id string) error {
	if s == nil || s.dir == "" {
		return nil
	}
	err := os.Remove(s.pathFor(id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// pruneOlderThan removes states whose FinishedAt predates the cutoff, keeping
// the newest keep newest-first states as a bounded history.
func (s *Store) pruneOlderThan(cutoff time.Time, keep int) error {
	states, err := s.List()
	if err != nil {
		return err
	}
	kept := 0
	for _, st := range states {
		if kept >= keep || (cutoff.After(st.UpdatedAt) && !st.FinishedAt.IsZero()) {
			if err := s.Delete(st.ID); err != nil {
				return err
			}
			continue
		}
		kept++
	}
	return nil
}
