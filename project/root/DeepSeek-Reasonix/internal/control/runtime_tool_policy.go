package control

import (
	"fmt"
	"sort"
	"strings"

	"reasonix/internal/permission"
)

// SessionToolDecisions returns APK/session-only tool policy overrides. The
// values are allow|ask|deny. Config policy remains authoritative: an existing
// explicit deny cannot be weakened by an APK allow.
func (c *Controller) SessionToolDecisions() map[string]string {
	out := map[string]string{}
	if c == nil {
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, d := range c.runtimeToolDecisions {
		out[name] = d.String()
	}
	return out
}

// SetSessionToolDecisions installs an ephemeral per-tool policy using the
// native Reasonix permission gate. Denied tools are also removed from the
// provider-visible schema to save prompt tokens, while capability resolution
// remains gated and cannot bypass the deny.
func (c *Controller) SetSessionToolDecisions(in map[string]string) error {
	if c == nil {
		return fmt.Errorf("controller is nil")
	}
	known := map[string]string{}
	for _, e := range c.AllToolContractEntries() {
		known[strings.ToLower(strings.TrimSpace(e.Name))] = e.Name
	}
	next := map[string]permission.Decision{}
	hidden := []string{}
	for raw, value := range in {
		key := strings.ToLower(strings.TrimSpace(raw))
		name, ok := known[key]
		if !ok {
			return fmt.Errorf("unknown tool: %s", strings.TrimSpace(raw))
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "allow":
			next[name] = permission.Allow
		case "ask":
			next[name] = permission.Ask
		case "deny":
			next[name] = permission.Deny
			hidden = append(hidden, name)
		default:
			return fmt.Errorf("invalid decision for %s: use allow, ask, or deny", name)
		}
	}
	sort.Strings(hidden)
	c.mu.Lock()
	c.runtimeToolDecisions = next
	c.mu.Unlock()
	if reg := c.mcp.registry(); reg != nil {
		reg.SetSessionHiddenTools(hidden)
	}
	effective := c.policyWithRuntimeToolDecisions(c.policy)
	c.approval.setPolicy(effective)
	if c.subagentGate != nil {
		c.subagentGate.SetPolicy(effective, c.approval.mode())
	}
	if c.writeAccess.interactive {
		c.refreshInteractiveGate()
	} else if c.executor != nil {
		c.executor.SetGate(c.newHeadlessGate(c.approval.mode()))
	}
	return nil
}

func (c *Controller) policyWithRuntimeToolDecisions(base permission.Policy) permission.Policy {
	c.mu.Lock()
	rules := make(map[string]permission.Decision, len(c.runtimeToolDecisions))
	for n, d := range c.runtimeToolDecisions {
		rules[n] = d
	}
	c.mu.Unlock()
	for name, d := range rules {
		rule := permission.Rule{Tool: name}
		switch d {
		case permission.Deny:
			base.Deny = append(base.Deny, rule)
		case permission.Ask:
			base.Ask = append(base.Ask, rule)
		case permission.Allow:
			base.SessionAllow = append(base.SessionAllow, rule)
		}
	}
	return base
}
