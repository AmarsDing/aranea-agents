package biz

import (
	"strings"
	"testing"
)

// Tests for replaceOrAppendPersona (PGO-1-BIZ-06).

func TestReplaceOrAppendPersona_NoSection(t *testing.T) {
	body := "# IDENTITY\n\nSome content here."
	persona := "I am a helpful assistant."

	out := replaceOrAppendPersona(body, persona)

	if !strings.Contains(out, "## Persona") {
		t.Fatal("expected ## Persona section to be appended")
	}
	if !strings.Contains(out, persona) {
		t.Fatalf("expected persona content in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Some content here") {
		t.Fatal("original content should be preserved")
	}
}

func TestReplaceOrAppendPersona_ReplaceExisting(t *testing.T) {
	body := "# IDENTITY\n\n## Persona\n\nOld persona text.\n\n## OtherSection\n\nother"
	newPersona := "Brand new persona."

	out := replaceOrAppendPersona(body, newPersona)

	if strings.Contains(out, "Old persona text") {
		t.Error("old persona text should be replaced")
	}
	if !strings.Contains(out, newPersona) {
		t.Errorf("new persona not found in output:\n%s", out)
	}
	if !strings.Contains(out, "## OtherSection") {
		t.Error("subsequent sections should be preserved")
	}
}

func TestReplaceOrAppendPersona_PreservesTrailingSections(t *testing.T) {
	body := "# IDENTITY\n\n## Persona\n\nOld.\n\n## Rules\n\nstay calm"
	out := replaceOrAppendPersona(body, "New persona")

	if !strings.Contains(out, "## Rules") {
		t.Error("## Rules section must survive persona replacement")
	}
	if !strings.Contains(out, "stay calm") {
		t.Error("content after ## Rules must survive")
	}
}

func TestReplaceOrAppendPersona_EmptyPersonaNoOp(t *testing.T) {
	body := "# IDENTITY\n\nfoo"
	out := replaceOrAppendPersona(body, "")
	// Empty persona: ## Persona section is still appended but the content area is empty.
	// The key invariant is that the original body content is preserved.
	if !strings.Contains(out, "foo") {
		t.Error("original body must be preserved")
	}
}

func TestReplaceOrAppendPersona_AtEOF(t *testing.T) {
	body := "# IDENTITY\n\n## Persona\n\nExisting persona."
	out := replaceOrAppendPersona(body, "Replacement")

	if strings.Contains(out, "Existing persona.") {
		t.Error("old content must be replaced")
	}
	if !strings.Contains(out, "Replacement") {
		t.Errorf("replacement not found:\n%s", out)
	}
}
