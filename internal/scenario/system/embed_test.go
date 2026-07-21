package systemprompts_test

import (
	"strings"
	"testing"

	systemprompts "aranea-agents/internal/scenario/system"
)

func TestEmbeddedSpiritPrompts(t *testing.T) {
	names, err := systemprompts.ListTopLevelMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"IDENTITY.md": true, "CAPABILITIES.md": true, "DECISION.md": true,
		"orchestrator.md": true, "dept_lead.md": true,
	}
	for _, n := range names {
		delete(want, n)
	}
	if len(want) > 0 {
		t.Fatalf("missing embedded prompts: %v (got %v)", want, names)
	}
	body, err := systemprompts.ReadMarkdown("IDENTITY.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "精灵") && len(body) < 32 {
		t.Fatalf("IDENTITY.md too short or unexpected: %q", body[:min(80, len(body))])
	}
	mem, err := systemprompts.ListSubdirMarkdown("memory")
	if err != nil || len(mem) == 0 {
		t.Fatalf("memory prompts: %v %v", mem, err)
	}
	skills, err := systemprompts.ListSubdirMarkdown("skills")
	if err != nil || len(skills) == 0 {
		t.Fatalf("skills prompts: %v %v", skills, err)
	}
}
