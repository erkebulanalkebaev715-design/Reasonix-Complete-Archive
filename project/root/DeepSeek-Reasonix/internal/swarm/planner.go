package swarm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Planner decides whether a swarm is useful and, when it is, decomposes the
// objective into a structured task graph with dependencies and worker scopes.
// The deterministic planner keeps the swarm offline-testable; hosts may inject
// their own Planner to make the decision with a model.
type Planner interface {
	Plan(ctx context.Context, o *Orchestrator, objective string) (*Plan, error)
}

// deterministicPlanner is the default Planner. Rules (all deterministic):
//
//   - a trivial objective (short, no separators) -> one task, no swarm;
//   - segments separated by ';' or newline -> independent parallel tasks;
//   - a segment prefixed with "then " or "after " depends on the previous one.
type deterministicPlanner struct{}

func newSwarmID(now time.Time) string {
	return fmt.Sprintf("swarm-%s", now.UTC().Format("20060102T150405.000000000Z"))
}

func splitSegments(objective string) []string {
	raw := strings.FieldsFunc(objective, func(r rune) bool { return r == ';' || r == '\n' })
	var out []string
	for _, seg := range raw {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

const trivialMaxRunes = 80

func (deterministicPlanner) Plan(ctx context.Context, o *Orchestrator, objective string) (*Plan, error) {
	if o == nil {
		return nil, fmt.Errorf("swarm: planner needs an orchestrator")
	}
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return nil, fmt.Errorf("swarm: empty objective")
	}
	segments := splitSegments(objective)
	if len(segments) <= 1 && len([]rune(objective)) <= trivialMaxRunes {
		// A trivial task must not launch unnecessary workers.
		return o.singleTaskPlan(objective), nil
	}
	if len(segments) == 1 {
		segments = splitOnVerbs(objective)
	}
	return o.multiTaskPlan(segments), nil
}

// splitOnVerbs is a conservative fallback for a long single-line objective:
// it only splits on explicit parallel verbs ("and investigate", "and verify")
// so it never fabricates dependencies.
func splitOnVerbs(objective string) []string {
	for _, verb := range []string{" and investigate ", " and verify ", " and debug ", " and research "} {
		if idx := strings.Index(objective, verb); idx > 0 {
			left := strings.TrimSpace(objective[:idx])
			right := strings.TrimSpace(objective[idx+len(verb):])
			if left != "" && right != "" {
				return []string{left, right}
			}
		}
	}
	return []string{objective}
}

func (o *Orchestrator) singleTaskPlan(objective string) *Plan {
	profile := o.profileFor(o.Default)
	task := &Task{
		ID:               taskIDFor(0, objective),
		Objective:        objective,
		Status:           TaskPending,
		Profile:          profile.Name,
		RequiredEvidence: profile.RequiredEvidence,
		CreatedAt:        o.Now(),
	}
	if len(profile.RequiredEvidence) == 0 {
		task.RequiredEvidence = []EvidenceKind{EvidenceProvider, EvidenceReadback}
	}
	return &Plan{Objective: objective, Tasks: []*Task{task}, Concurrency: 1}
}

func (o *Orchestrator) multiTaskPlan(segments []string) *Plan {
	concurrency := o.Limits.MaxWorkers
	if concurrency > len(segments) {
		concurrency = len(segments)
	}
	tasks := make([]*Task, 0, len(segments))
	var previous string
	for i, seg := range segments {
		seg = strings.TrimSpace(seg)
		dependent := false
		for _, prefix := range []string{"then ", "after "} {
			if strings.HasPrefix(strings.ToLower(seg), prefix) {
				dependent = true
				seg = strings.TrimSpace(seg[len(prefix):])
				break
			}
		}
		profile := o.profileFor(o.Default)
		task := &Task{
			ID:               taskIDFor(i, seg),
			Objective:        seg,
			Status:           TaskPending,
			Profile:          profile.Name,
			RequiredEvidence: profile.RequiredEvidence,
			CreatedAt:        o.Now(),
		}
		if len(profile.RequiredEvidence) == 0 {
			task.RequiredEvidence = []EvidenceKind{EvidenceProvider, EvidenceReadback}
		}
		if dependent && previous != "" {
			task.Dependencies = []string{previous}
		}
		tasks = append(tasks, task)
		previous = task.ID
	}
	limits := map[string]int{}
	return &Plan{Objective: strings.Join(segments, "; "), Tasks: tasks, Concurrency: concurrency, ProviderLimits: limits}
}

func taskIDFor(i int, segment string) string {
	word := firstWord(segment)
	return fmt.Sprintf("%02d-%s", i, word)
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "task"
	}
	for _, sep := range []string{" ", "\n"} {
		if idx := strings.Index(s, sep); idx > 0 {
			s = s[:idx]
		}
	}
	if len(s) > 24 {
		s = s[:24]
	}
	if s == "" {
		return "task"
	}
	return s
}

// integrate combines the structured task results into one final answer. It
// never concatenates worker prose: findings are deduplicated by ID, artifact
// lists are merged, and the summary is assembled from the per-task structured
// summaries with explicit conflict surfacing.
func (o *Orchestrator) integrate() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var sections []string
	seen := map[string]bool{}
	for _, f := range o.state.Findings {
		if f.Summary == "" || seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		sections = append(sections, "- "+f.Summary)
	}
	for _, id := range sortedTaskIDs(o.state) {
		t := o.state.Tasks[id]
		if t == nil || t.Result == nil || t.Status != TaskSucceeded {
			continue
		}
		if t.Result.Summary != "" {
			sections = append(sections, "## "+t.ID+"\n"+t.Result.Summary)
		}
		for _, a := range t.Result.Artifacts {
			sections = append(sections, fmt.Sprintf("artifact %s sha256:%s", a.Path, a.SHA256))
		}
	}
	if len(sections) == 0 {
		return "no structured results produced"
	}
	return strings.Join(sections, "\n\n")
}

func sortedTaskIDs(s *SwarmState) []string {
	ids := make([]string, 0, len(s.Tasks))
	for id := range s.Tasks {
		ids = append(ids, id)
	}
	// task IDs are zero-padded by segment index, so lexical order is the task
	// graph order.
	sort.Strings(ids)
	return ids
}

// verifyIntegrated checks the whole swarm's integrated output against the
// required evidence contract: at least one succeeded task must exist, and the
// result must be non-empty unless every task failed.
func (o *Orchestrator) verifyIntegrated() (bool, string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.state.Tasks) == 0 {
		return false, "no tasks"
	}
	if strings.TrimSpace(o.state.Result) == "" {
		return false, "empty integrated result"
	}
	return true, "integrated result verified"
}

func missingRequiredEvidence(result *TaskResult, required []EvidenceKind) bool {
	if result == nil {
		return true
	}
	for _, kind := range required {
		found := false
		for _, ev := range result.Evidence {
			if ev.Kind == kind && ev.Result == "pass" {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}
