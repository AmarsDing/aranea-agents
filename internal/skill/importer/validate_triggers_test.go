package importer

import (
	"testing"
)

// P1-3：ValidateSkillPackage 必须同时返回 frontmatter 中的 triggers，
// 供 watcher/导入链路写入 metadata envelope 供运行时路由使用。
func TestValidateSkillPackage_ReturnsTriggers(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\nname: expense-helper\ndescription: 当需要处理报销流程时使用\ntriggers: [报销, invoice, expense report]\n---\n# Expense\n\nbody"),
	}
	candidate, tags, triggers := ValidateSkillPackage(files, "expense-helper", nil, true)
	if candidate.ValidationStatus != "pass" {
		t.Fatalf("expected pass, got %q (%+v)", candidate.ValidationStatus, candidate.Blocks)
	}
	if len(tags) != 0 {
		t.Fatalf("expected no tags, got %v", tags)
	}
	want := []string{"报销", "invoice", "expense report"}
	if len(triggers) != len(want) {
		t.Fatalf("triggers = %v, want %v", triggers, want)
	}
	for i := range want {
		if triggers[i] != want[i] {
			t.Fatalf("triggers[%d] = %q, want %q", i, triggers[i], want[i])
		}
	}
}

func TestValidateSkillPackage_NoTriggers(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\nname: plain\ndescription: 当需要时使用\n---\n# Plain\n\nbody"),
	}
	_, _, triggers := ValidateSkillPackage(files, "plain", nil, true)
	if len(triggers) != 0 {
		t.Fatalf("expected no triggers, got %v", triggers)
	}
}
