package swarm

import (
	"sort"
	"strings"
)

// readOnlyTools is the default read/research tool surface. Write tools are
// excluded; workers that must mutate get an explicit ownership scope and the
// write tools in their allowed set.
var readOnlyTools = []string{
	"bash", "read_file", "grep", "glob", "code_index", "todo_write",
	"complete_step", "history", "recall",
}

var writeTools = []string{"edit_file", "write_file", "multi_edit", "move_file", "apply_patch"}

// DefaultProfiles returns the generic built-in worker profiles. The swarm
// architecture does not depend on these exact names; any profile string in a
// plan resolves against the profile map, and unknown names fall back to the
// default profile. Evidence requirements are the baseline host contract; a
// plan may tighten them per task (e.g. require compile or unit-test evidence
// for coding tasks).
func DefaultProfiles() map[string]Profile {
	return map[string]Profile{
		"researcher": {
			Name:             "researcher",
			Instructions:     "Investigate the bounded objective using read-only tools. Return structured findings and references; do not modify files.",
			AllowedTools:     readOnlyTools,
			ReadOnly:         true,
			MaxSteps:         20,
			RequiredEvidence: []EvidenceKind{EvidenceProvider, EvidenceReadback},
		},
		"debugger": {
			Name:             "debugger",
			Instructions:     "Diagnose the bounded objective by inspecting source and running targeted commands. Return the root cause, evidence, and the exact files/lines involved. Only write within your assigned ownership scope.",
			AllowedTools:     append(append([]string{}, readOnlyTools...), writeTools...),
			MaxSteps:         25,
			RequiredEvidence: []EvidenceKind{EvidenceProvider, EvidenceReadback},
		},
		"verifier": {
			Name:             "verifier",
			Instructions:     "Verify the given claim against the repository. Run read-only checks and report pass/fail with the exact evidence (commands, results, line numbers).",
			AllowedTools:     readOnlyTools,
			ReadOnly:         true,
			MaxSteps:         20,
			RequiredEvidence: []EvidenceKind{EvidenceProvider, EvidenceReadback},
		},
		"integrator": {
			Name:             "integrator",
			Instructions:     "Combine the supplied structured findings into a single coherent answer. Resolve contradictions explicitly. Do not invent evidence.",
			AllowedTools:     append(append([]string{}, readOnlyTools...), writeTools...),
			MaxSteps:         15,
			RequiredEvidence: []EvidenceKind{EvidenceProvider, EvidenceReadback},
		},
	}
}

// normalizeProfileTools validates the allowed-tool set against the actual
// registry, dropping unknown names so a typo can never widen a worker's tool
// surface through a different name.
func normalizeProfileTools(allowed []string, have func(name string) bool) []string {
	if len(allowed) == 0 {
		return nil // nil = all tools visible, still scoped by ownership
	}
	seen := map[string]bool{}
	var out []string
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] || !have(name) {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
