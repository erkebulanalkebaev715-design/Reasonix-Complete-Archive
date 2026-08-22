package agent

import (
	"fmt"
	"strings"

	"reasonix/internal/tool"
)

// strictBudgetDelegationTools are provider-spawning tools whose child/model
// requests do not yet share the executor's pre-call reservation ledger. Hard
// budget mode blocks them at execution (including proxy-resolved targets) so a
// child agent cannot bypass the promised spend ceiling. Inline/read-only local
// skill inspection remains available through read_skill.
var strictBudgetDelegationTools = map[string]bool{
	"task":            true,
	"read_only_task":  true,
	"parallel_tasks":  true,
	"fleet":           true,
	"run_skill":       true,
	"read_only_skill": true,
	"explore":         true,
	"research":        true,
	"review":          true,
	"security_review": true,
	"security-review": true,
}

func strictBudgetDelegationName(name string) bool {
	return strictBudgetDelegationTools[strings.ToLower(strings.TrimSpace(name))]
}

func (a *Agent) strictBudgetDelegationBlock(visible tool.Tool, resolved *tool.ResolvedCall) (toolOutcome, bool) {
	if a == nil || !a.StrictPreCallBudget() {
		return toolOutcome{}, false
	}
	name := ""
	if resolved != nil {
		name = strings.TrimSpace(resolved.TargetName)
		if name == "" && resolved.Target != nil {
			name = resolved.Target.Name()
		}
	}
	if name == "" && visible != nil {
		name = visible.Name()
	}
	if !strictBudgetDelegationName(name) {
		return toolOutcome{}, false
	}
	msg := fmt.Sprintf("blocked: %s is disabled while hard pre-call budget mode is active; use the current agent or disable hardStop", name)
	return toolOutcome{output: msg, blocked: true, errMsg: "blocked by hard budget provider policy"}, true
}
