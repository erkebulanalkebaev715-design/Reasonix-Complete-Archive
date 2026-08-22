package efficiency

import (
	"strings"
	"testing"
)

func TestReduceBuildLogExtractsErrorContext(t *testing.T) {
	raw := strings.Repeat("noise\n", 100) + "before\nerror: unresolved reference Foo\nafter\n" + strings.Repeat("tail\n", 100)
	s := ReduceBuildLog(raw, DefaultLogReduceConfig())
	if !strings.Contains(s.Text, "unresolved reference Foo") || strings.Contains(s.Text, strings.Repeat("noise", 5)) {
		t.Fatalf("summary=%q", s.Text)
	}
	if s.OriginalLines <= s.SelectedLines {
		t.Fatalf("not reduced: %+v", s)
	}
}

func TestReduceBuildLogTailFallback(t *testing.T) {
	s := ReduceBuildLog("a\nb\nc\nd\n", LogReduceConfig{MaxLines: 2, Context: 0, MaxBytes: 100})
	if !strings.Contains(s.Text, "d") {
		t.Fatalf("summary=%q", s.Text)
	}
}
