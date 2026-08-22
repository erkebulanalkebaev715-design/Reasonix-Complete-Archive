package efficiency

import "testing"

func TestProjectProfileNormalizesAndValidates(t *testing.T) {
	m := NewProjectProfileManager()
	got, err := m.Configure(ProjectProfile{Name: "  Space Game ", Mode: AgentModeChat, ToolPacks: []string{"Basic", "basic", "VERIFY"}, LiveDetail: LiveDetailMetadata})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Space Game" || got.Mode != AgentModeChat || got.LiveDetail != LiveDetailMetadata {
		t.Fatalf("profile=%+v", got)
	}
	if len(got.ToolPacks) != 2 || got.ToolPacks[0] != "basic" || got.ToolPacks[1] != "verify" {
		t.Fatalf("packs=%v", got.ToolPacks)
	}
	if _, err := m.Configure(ProjectProfile{Mode: "root", LiveDetail: LiveDetailProject}); err == nil {
		t.Fatal("invalid mode accepted")
	}
}
