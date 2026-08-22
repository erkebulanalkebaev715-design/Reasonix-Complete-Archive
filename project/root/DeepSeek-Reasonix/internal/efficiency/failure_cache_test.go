package efficiency

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFailureCachePersistsAndSkipsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failures.jsonl")
	c, err := OpenFailureCache(path)
	if err != nil {
		t.Fatal(err)
	}
	rec := FailureRecord{Fingerprint: "abc", Environment: "go1.26/linux-arm64", Files: []string{"b.go", "a.go"}, FixHint: "repair import", Verified: true}
	if err := c.PutVerified(rec); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("{corrupt}\n")
	_ = f.Close()
	c2, err := OpenFailureCache(path)
	if err != nil {
		t.Fatal(err)
	}
	got, score, ok := c2.Lookup("abc", "go1.26/linux-arm64", []string{"a.go", "b.go"})
	if !ok || got.FixHint != "repair import" || score < .99 {
		t.Fatalf("got=%+v score=%v ok=%v", got, score, ok)
	}
	if c2.Stats().CorruptSkipped != 1 {
		t.Fatalf("stats=%+v", c2.Stats())
	}
}

func TestFailureCacheNeverStoresUnverified(t *testing.T) {
	c, _ := OpenFailureCache("")
	_ = c.PutVerified(FailureRecord{Fingerprint: "x", FixHint: "bad", Verified: false})
	if c.Stats().Records != 0 {
		t.Fatal("unverified fix was cached")
	}
}
