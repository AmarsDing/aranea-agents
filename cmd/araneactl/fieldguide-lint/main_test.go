package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractScopes(t *testing.T) {
	dir := t.TempDir()
	goPath := filepath.Join(dir, "field_guides.go")
	tsPath := filepath.Join(dir, "fieldGuides.ts")

	if err := os.WriteFile(goPath, []byte(`
package biz

type FieldScope string

const (
	ScopeCategoryIndustry FieldScope = "category.industry"
	ScopeAgentFile        FieldScope = "agent.file"
	ScopeSpecExtract      FieldScope = "spec_extract"
)
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(tsPath, []byte(`
export type FieldScope =
  | 'category.industry'
  | 'agent.file'

// A line with | 'not.a.scope' outside the union must not be collected.
const example = "| 'not.a.scope'"
`), 0644); err != nil {
		t.Fatal(err)
	}

	goScopes, err := extractGoScopes(goPath)
	if err != nil {
		t.Fatal(err)
	}
	tsScopes, err := extractTSScopes(tsPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, scope := range []string{"category.industry", "agent.file", "spec_extract"} {
		if !goScopes[scope] {
			t.Fatalf("expected Go scope %q", scope)
		}
	}
	for _, scope := range []string{"category.industry", "agent.file"} {
		if !tsScopes[scope] {
			t.Fatalf("expected TS scope %q", scope)
		}
	}
	if tsScopes["not.a.scope"] {
		t.Fatalf("unexpected scope collected outside FieldScope union")
	}
}
