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
