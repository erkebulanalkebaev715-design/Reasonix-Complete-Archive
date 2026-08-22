package swarm

import (
	"testing"
	"time"
)

func TestStoreRoundTripsSwarmState(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	now := time.Unix(1_700_000_000, 0).UTC()
	state := &SwarmState{
		ID:        "swarm-test",
		Objective: "Investigate A; Investigate B",
		Status:    StatusDone,
		CreatedAt: now,
		UpdatedAt: now,
		Tasks: map[string]*Task{
			"01-A": {ID: "01-A", Objective: "Investigate A", Status: TaskSucceeded, Model: "mock/deepseek-v4-flash"},
			"02-B": {ID: "02-B", Objective: "Investigate B", Status: TaskCancelled},
		},
		Result: "integrated result",
	}
	if err := s.Save(state); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.Load("swarm-test")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("loaded nil state")
	}
	if got.Status != StatusDone || len(got.Tasks) != 2 {
		t.Fatalf("loaded state = %+v", got)
	}
	if got.Tasks["01-A"].Status != TaskSucceeded {
		t.Fatalf("task 01-A status = %s", got.Tasks["01-A"].Status)
	}
	list, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d states, want 1", len(list))
	}
	if err := s.Delete("swarm-test"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got, _ := s.Load("swarm-test"); got != nil {
		t.Fatal("state survived delete")
	}
}

func TestStoreToleratesMissingState(t *testing.T) {
	s := NewStore(t.TempDir())
	got, err := s.Load("nope")
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got != nil {
		t.Fatalf("missing state loaded: %+v", got)
	}
	if err := s.Delete("nope"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Fatalf("list = %d, want 0", len(list))
	}
}

func TestStoreNilIsNoop(t *testing.T) {
	var s *Store
	if err := s.Save(&SwarmState{ID: "x"}); err != nil {
		t.Fatalf("nil store save: %v", err)
	}
	if _, err := s.Load("x"); err != nil {
		t.Fatalf("nil store load: %v", err)
	}
}
