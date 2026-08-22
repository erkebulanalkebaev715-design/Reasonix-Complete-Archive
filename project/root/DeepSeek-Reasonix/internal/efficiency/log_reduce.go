package efficiency

import (
	"regexp"
	"strings"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// LogSummary is deliberately small enough to feed to an LLM while the complete
// log remains local on disk/tool history.
type LogSummary struct {
	OriginalLines int      `json:"originalLines"`
	SelectedLines int      `json:"selectedLines"`
	Truncated     bool     `json:"truncated"`
	Text          string   `json:"text"`
	Signals       []string `json:"signals,omitempty"`
}

type LogReduceConfig struct {
	MaxLines int
	Context  int
	MaxBytes int
}

func DefaultLogReduceConfig() LogReduceConfig {
	return LogReduceConfig{MaxLines: 80, Context: 2, MaxBytes: 16 * 1024}
}

// ReduceBuildLog keeps root-cause/error blocks and local context. If nothing
// looks like an error, it falls back to the tail because build systems often
// print the only useful status there.
func ReduceBuildLog(raw string, cfg LogReduceConfig) LogSummary {
	if cfg.MaxLines <= 0 {
		cfg.MaxLines = 80
	}
	if cfg.Context < 0 {
		cfg.Context = 0
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 16 * 1024
	}
	clean := ansiRE.ReplaceAllString(strings.ReplaceAll(raw, "\r\n", "\n"), "")
	lines := strings.Split(clean, "\n")
	selected := map[int]struct{}{}
	signalSet := map[string]struct{}{}
	for i, line := range lines {
		l := strings.ToLower(line)
		sig := ""
		switch {
		case strings.Contains(l, "caused by:"):
			sig = "caused_by"
		case strings.Contains(l, "fatal error") || strings.Contains(l, " fatal:"):
			sig = "fatal"
		case strings.Contains(l, "error:") || strings.HasPrefix(strings.TrimSpace(l), "error "):
			sig = "error"
		case strings.Contains(l, "failed") && (strings.Contains(l, "task") || strings.Contains(l, "build") || strings.Contains(l, "test")):
			sig = "failed"
		case strings.Contains(l, "panic:") || strings.Contains(l, "exception"):
			sig = "exception"
		case strings.Contains(l, "undefined") || strings.Contains(l, "unresolved reference"):
			sig = "symbol"
		}
		if sig == "" {
			continue
		}
		signalSet[sig] = struct{}{}
		for j := i - cfg.Context; j <= i+cfg.Context; j++ {
			if j >= 0 && j < len(lines) {
				selected[j] = struct{}{}
			}
		}
	}
	idx := make([]int, 0, len(selected))
	for i := range selected {
		idx = append(idx, i)
	}
	// insertion sort keeps package dependency-free and idx is tiny in practice
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && idx[j] < idx[j-1]; j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	if len(idx) == 0 {
		start := len(lines) - minInt(cfg.MaxLines, len(lines))
		for i := start; i < len(lines); i++ {
			idx = append(idx, i)
		}
	}
	truncated := false
	if len(idx) > cfg.MaxLines {
		idx = idx[:cfg.MaxLines]
		truncated = true
	}
	var b strings.Builder
	kept := 0
	for _, i := range idx {
		row := lines[i]
		candidate := row + "\n"
		if b.Len()+len(candidate) > cfg.MaxBytes {
			truncated = true
			break
		}
		b.WriteString(candidate)
		kept++
	}
	sigs := make([]string, 0, len(signalSet))
	for s := range signalSet {
		sigs = append(sigs, s)
	}
	for i := 1; i < len(sigs); i++ {
		for j := i; j > 0 && sigs[j] < sigs[j-1]; j-- {
			sigs[j], sigs[j-1] = sigs[j-1], sigs[j]
		}
	}
	return LogSummary{OriginalLines: len(lines), SelectedLines: kept, Truncated: truncated || kept < len(idx), Text: strings.TrimSpace(b.String()), Signals: sigs}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
