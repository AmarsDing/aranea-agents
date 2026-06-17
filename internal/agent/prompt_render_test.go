package agent

import (
	"strings"
	"testing"
)

func TestRenderPromptTemplate_BasicSubstitution(t *testing.T) {
	tmpl := "Hello {name}, your role is {{role}}."
	vars := map[string]string{
		"name": "Alice",
		"role": "engineer",
	}
	got, err := RenderPromptTemplate(tmpl, vars)
	if err != nil {
		t.Fatalf("RenderPromptTemplate returned error: %v", err)
	}
	want := "Hello Alice, your role is engineer."
	if got != want {
		t.Errorf("RenderPromptTemplate(%q, %v) = %q, want %q", tmpl, vars, got, want)
	}
}

func TestRenderPromptTemplate_MissingVariablesPreserved(t *testing.T) {
	// SyntaxMixedBrace default: PreserveUnknown — unresolved placeholders
	// remain in the output so later stages can still see them.
	tmpl := "Hello {name}, your level is {level}."
	vars := map[string]string{
		"name": "Bob",
	}
	got, err := RenderPromptTemplate(tmpl, vars)
	if err != nil {
		t.Fatalf("RenderPromptTemplate returned error: %v", err)
	}
	if !strings.Contains(got, "Bob") {
		t.Errorf("expected rendered output to contain 'Bob', got %q", got)
	}
	if !strings.Contains(got, "{level}") {
		t.Errorf("expected unresolved placeholder {level} to be preserved, got %q", got)
	}
}

func TestRenderPromptTemplate_EmptyVars(t *testing.T) {
	tmpl := "No variables here."
	got, err := RenderPromptTemplate(tmpl, nil)
	if err != nil {
		t.Fatalf("RenderPromptTemplate returned error: %v", err)
	}
	if got != tmpl {
		t.Errorf("RenderPromptTemplate with nil vars = %q, want %q", got, tmpl)
	}
}

func TestRenderCapabilityCue(t *testing.T) {
	tmpl := "## Runtime capability policy (system)\n- Subagents: enabled; max_concurrency={max_concurrency}, max_depth={max_depth}\n- Tools: enabled; profile=\"{profile}\""
	vars := map[string]string{
		"max_concurrency": "3",
		"max_depth":       "2",
		"profile":         "default",
	}
	got, err := RenderCapabilityCue(tmpl, vars)
	if err != nil {
		t.Fatalf("RenderCapabilityCue returned error: %v", err)
	}
	if !strings.Contains(got, "max_concurrency=3") {
		t.Errorf("expected rendered output to contain 'max_concurrency=3', got %q", got)
	}
	if !strings.Contains(got, "max_depth=2") {
		t.Errorf("expected rendered output to contain 'max_depth=2', got %q", got)
	}
	if !strings.Contains(got, `profile="default"`) {
		t.Errorf("expected rendered output to contain profile value, got %q", got)
	}
}

func TestRenderCapabilityCue_MissingVarsPreserved(t *testing.T) {
	tmpl := "- Tools: enabled; profile={profile}"
	got, err := RenderCapabilityCue(tmpl, nil)
	if err != nil {
		t.Fatalf("RenderCapabilityCue returned error: %v", err)
	}
	if !strings.Contains(got, "{profile}") {
		t.Errorf("expected unresolved {profile} preserved, got %q", got)
	}
}
