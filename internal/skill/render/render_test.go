package render

import (
	"strings"
	"testing"

	"aranea-agents/internal/skill/manifest"
)

func TestSkillGuidance_NameOnly(t *testing.T) {
	m := manifest.Manifest{Name: "MySkill"}
	got := SkillGuidance(m, RenderOptions{})
	if !strings.HasPrefix(got, "## MySkill\n") {
		t.Fatalf("expected heading, got %q", got)
	}
}

func TestSkillGuidance_DescriptionOnly(t *testing.T) {
	m := manifest.Manifest{Description: "A helpful skill."}
	got := SkillGuidance(m, RenderOptions{})
	if !strings.Contains(got, "A helpful skill.\n") {
		t.Fatalf("expected description, got %q", got)
	}
	if strings.Contains(got, "## ") {
		t.Fatal("should not have heading when Name is empty")
	}
}

func TestSkillGuidance_BodyOnly(t *testing.T) {
	m := manifest.Manifest{Body: "Do something useful."}
	got := SkillGuidance(m, RenderOptions{})
	if got != "Do something useful." {
		t.Fatalf("got %q", got)
	}
}

func TestSkillGuidance_AllFields(t *testing.T) {
	m := manifest.Manifest{
		Name:        "TestSkill",
		Description: "Desc",
		Body:        "Step 1",
	}
	got := SkillGuidance(m, RenderOptions{})
	if !strings.HasPrefix(got, "## TestSkill\n") {
		t.Fatalf("missing heading, got %q", got)
	}
	if !strings.Contains(got, "Desc\n") {
		t.Fatalf("missing description, got %q", got)
	}
	if !strings.Contains(got, "Step 1") {
		t.Fatalf("missing body, got %q", got)
	}
}

func TestSkillGuidance_VariableSubstitution(t *testing.T) {
	m := manifest.Manifest{
		Name: "VarSkill",
		Body: "Hello {{name}}, welcome to {{place}}.",
	}
	opts := RenderOptions{Variables: map[string]string{
		"name":  "Alice",
		"place": "Wonderland",
	}}
	got := SkillGuidance(m, opts)
	if !strings.Contains(got, "Hello Alice, welcome to Wonderland.") {
		t.Fatalf("variables not substituted, got %q", got)
	}
}

func TestSkillGuidance_VariableSubstitution_Partial(t *testing.T) {
	m := manifest.Manifest{Body: "{{a}} and {{b}}"}
	opts := RenderOptions{Variables: map[string]string{"a": "X"}}
	got := SkillGuidance(m, opts)
	if !strings.Contains(got, "X and {{b}}") {
		t.Fatalf("partial substitution failed, got %q", got)
	}
}

func TestSkillGuidance_EmptyManifest(t *testing.T) {
	got := SkillGuidance(manifest.Manifest{}, RenderOptions{})
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestSkillGuidance_NilVariables(t *testing.T) {
	m := manifest.Manifest{Body: "no vars {{x}}"}
	got := SkillGuidance(m, RenderOptions{})
	if !strings.Contains(got, "{{x}}") {
		t.Fatalf("unresolved placeholder should remain, got %q", got)
	}
}

// 调用契约 7.4 allowed-tools 软约束的 L0 信息面：skill frontmatter 的
// tools: 声明经 manifest.Parse → AI 优化渲染进路由 cue（"Tools: a, b"），
// 模型据此优先使用声明工具。full 模式不输出该行（L1 注入的原始 SKILL.md
// 已含 frontmatter，不重复）。
func TestSkillGuidance_AIOptimized_ToolsLine(t *testing.T) {
	m := manifest.Manifest{
		Name:  "ToolSkill",
		Tools: []string{"exec_command", "read_file"},
		Body:  "## 步骤\nDo it",
	}
	got := SkillGuidance(m, RenderOptions{Mode: ModeAIOptimized})
	if !strings.Contains(got, "Tools: exec_command, read_file\n") {
		t.Fatalf("ai_optimized must surface declared tools, got %q", got)
	}

	// 未声明 tools 时不输出该行（避免噪音）。
	m.Tools = nil
	got = SkillGuidance(m, RenderOptions{Mode: ModeAIOptimized})
	if strings.Contains(got, "Tools:") {
		t.Fatalf("no tools line expected when undeclared, got %q", got)
	}

	// full 模式不渲染 Tools 行。
	m.Tools = []string{"exec_command"}
	got = SkillGuidance(m, RenderOptions{Mode: ModeFull})
	if strings.Contains(got, "Tools:") {
		t.Fatalf("full mode must not emit tools line, got %q", got)
	}
}
