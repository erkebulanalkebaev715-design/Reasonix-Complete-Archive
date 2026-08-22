package efficiency

import "testing"

func TestPatchGovernorAllowsSmallFix(t *testing.T) {
	r, err := CheckNumstat("10\t2\ta.go\n3\t0\tb.go\n", DefaultPatchPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Allowed || r.Files != 2 || r.ChangedLines != 15 {
		t.Fatalf("%+v", r)
	}
}
func TestPatchGovernorBlocksOverscope(t *testing.T) {
	r, err := CheckNumstat("400\t0\ta.go\n200\t0\tb.go\n", DefaultPatchPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if r.Allowed {
		t.Fatalf("%+v", r)
	}
}
