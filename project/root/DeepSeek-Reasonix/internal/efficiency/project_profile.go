package efficiency

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// AgentMode controls how much execution authority the APK grants to the same
// Reasonix agent. Chat mode keeps the conversation/model/session intact but
// hides mutating tools from the provider surface; Agent mode restores the
// configured tool-pack/manual policy.
type AgentMode string

const (
	AgentModeChat  AgentMode = "chat"
	AgentModeAgent AgentMode = "agent"
)

const (
	LiveDetailMetadata = "metadata"
	LiveDetailProject  = "project"
)

// ProjectProfile is intentionally provider-agnostic. The APK can persist this
// object and re-apply it after restarting reasonix serve in another workspace.
type ProjectProfile struct {
	Name       string    `json:"name,omitempty"`
	Mode       AgentMode `json:"mode"`
	ToolPacks  []string  `json:"toolPacks,omitempty"`
	LiveDetail string    `json:"liveDetail"`
}

// ProjectProfileManager owns only APK/session policy. It does not duplicate
// Reasonix memory, model routing, budget or workspace state.
type ProjectProfileManager struct {
	mu      sync.RWMutex
	profile ProjectProfile
}

func DefaultProjectProfile() ProjectProfile {
	return ProjectProfile{Mode: AgentModeAgent, LiveDetail: LiveDetailProject}
}

func NewProjectProfileManager() *ProjectProfileManager {
	return &ProjectProfileManager{profile: DefaultProjectProfile()}
}

func normalizePackNames(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.ToLower(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func ValidateProjectProfile(p ProjectProfile) error {
	switch p.Mode {
	case AgentModeChat, AgentModeAgent:
	default:
		return fmt.Errorf("mode must be chat or agent")
	}
	switch strings.ToLower(strings.TrimSpace(p.LiveDetail)) {
	case LiveDetailMetadata, LiveDetailProject:
	default:
		return fmt.Errorf("liveDetail must be metadata or project")
	}
	if len(p.Name) > 120 {
		return fmt.Errorf("project name is too long")
	}
	if len(p.ToolPacks) > 32 {
		return fmt.Errorf("too many tool packs")
	}
	return nil
}

func (m *ProjectProfileManager) Configure(p ProjectProfile) (ProjectProfile, error) {
	if m == nil {
		return ProjectProfile{}, fmt.Errorf("project profile manager is nil")
	}
	p.Name = strings.TrimSpace(p.Name)
	p.LiveDetail = strings.ToLower(strings.TrimSpace(p.LiveDetail))
	if p.LiveDetail == "" {
		p.LiveDetail = LiveDetailProject
	}
	if p.Mode == "" {
		p.Mode = AgentModeAgent
	}
	p.ToolPacks = normalizePackNames(p.ToolPacks)
	if err := ValidateProjectProfile(p); err != nil {
		return ProjectProfile{}, err
	}
	m.mu.Lock()
	m.profile = p
	m.mu.Unlock()
	return m.Snapshot(), nil
}

func (m *ProjectProfileManager) Snapshot() ProjectProfile {
	if m == nil {
		return DefaultProjectProfile()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := m.profile
	out.ToolPacks = append([]string(nil), m.profile.ToolPacks...)
	return out
}
