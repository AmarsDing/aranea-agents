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

// 2026-07-25 Fix 4：DECISION.md 必须保留「需求不明先澄清、禁止组队」约束。
// 19:29 根因链起点：需求存在阻塞性歧义时精灵直接组队，团队无法向用户提问，
// 只能空转或编造产出。该约束是组队决策咽喉点的唯一提示层防线。
func TestDecisionPrompt_RequiresClarificationBeforeTeaming(t *testing.T) {
	body, err := systemprompts.ReadMarkdown("DECISION.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"阻塞性歧义", "禁止组队", "需求不明时组队"} {
		if !strings.Contains(body, want) {
			t.Fatalf("DECISION.md should contain the clarification-first rule %q", want)
		}
	}
}
