package efficiency

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

type PatchPolicy struct {
	MaxFiles           int `json:"maxFiles"`
	MaxChangedLines    int `json:"maxChangedLines"`
	MaxSingleFileLines int `json:"maxSingleFileLines"`
}

type PatchReport struct {
	Files            int    `json:"files"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	ChangedLines     int    `json:"changedLines"`
	LargestFileLines int    `json:"largestFileLines"`
	Allowed          bool   `json:"allowed"`
	Reason           string `json:"reason,omitempty"`
}

func DefaultPatchPolicy() PatchPolicy {
	return PatchPolicy{MaxFiles: 6, MaxChangedLines: 500, MaxSingleFileLines: 300}
}

// CheckNumstat parses `git diff --numstat`. It is intentionally based on Git's
// own output rather than a second diff implementation/library.
func CheckNumstat(numstat string, policy PatchPolicy) (PatchReport, error) {
	if policy.MaxFiles <= 0 {
		policy.MaxFiles = 6
	}
	if policy.MaxChangedLines <= 0 {
		policy.MaxChangedLines = 500
	}
	if policy.MaxSingleFileLines <= 0 {
		policy.MaxSingleFileLines = 300
	}
	var r PatchReport
	s := bufio.NewScanner(strings.NewReader(numstat))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			return r, fmt.Errorf("invalid numstat line %q", line)
		}
		// Binary files use '-' counts: treat them as one changed file but do not
		// invent line counts. They still contribute to the file-scope limit.
		add, del := 0, 0
		var err error
		if parts[0] != "-" {
			add, err = strconv.Atoi(parts[0])
			if err != nil {
				return r, err
			}
		}
		if parts[1] != "-" {
			del, err = strconv.Atoi(parts[1])
			if err != nil {
				return r, err
			}
		}
		changed := add + del
		r.Files++
		r.Additions += add
		r.Deletions += del
		r.ChangedLines += changed
		if changed > r.LargestFileLines {
			r.LargestFileLines = changed
		}
	}
	if err := s.Err(); err != nil {
		return r, err
	}
	r.Allowed = true
	switch {
	case r.Files > policy.MaxFiles:
		r.Allowed = false
		r.Reason = "patch touches too many files; split the repair"
	case r.ChangedLines > policy.MaxChangedLines:
		r.Allowed = false
		r.Reason = "patch changes too many lines; split the repair"
	case r.LargestFileLines > policy.MaxSingleFileLines:
		r.Allowed = false
		r.Reason = "single-file patch is too large; require explicit large-refactor mode"
	}
	return r, nil
}
