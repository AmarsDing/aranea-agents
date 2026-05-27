package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestR12_Violation(t *testing.T) {
	// Read the fixture.
	fixture, err := os.ReadFile(filepath.Join("testdata", "r12_violation.go.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Write to a temp .go file so parseFile can read it.
	tmp, err := os.CreateTemp("", "r12-violation-*.go")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(fixture); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	tmp.Close()

	_, imports, err := parseFile(tmp.Name())
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}

	// Simulate the file being in cmd/aranea/ so R12 applies.
	rel := "cmd/aranea/badcli.go"
	vs := r12CLINoBackendImport(rel, imports)
	if len(vs) == 0 {
		t.Error("expected at least one R12 violation, got none")
	}
	for _, v := range vs {
		if v.rule != "R12" {
			t.Errorf("expected rule R12, got %s", v.rule)
		}
	}
}

func TestR12_NoViolation_NormalCLI(t *testing.T) {
	imports := []string{
		"aranea-agents/internal/cli/client",
		"aranea-agents/internal/cli/config",
		"github.com/spf13/cobra",
	}
	rel := "internal/cli/cmd/agent.go"
	vs := r12CLINoBackendImport(rel, imports)
	if len(vs) != 0 {
		t.Errorf("expected no violations for clean CLI file, got %d: %v", len(vs), vs)
	}
}

func TestR12_NoViolation_NonCLIFile(t *testing.T) {
	// A file in internal/service should be ignored by R12.
	imports := []string{
		"aranea-agents/internal/biz",
	}
	rel := "internal/service/admin.go"
	vs := r12CLINoBackendImport(rel, imports)
	if len(vs) != 0 {
		t.Errorf("expected no violations for non-CLI file, got %d", len(vs))
	}
}

func TestR12_ViolationService(t *testing.T) {
	imports := []string{
		"aranea-agents/internal/service",
	}
	rel := "cmd/aranea/main.go"
	vs := r12CLINoBackendImport(rel, imports)
	if len(vs) == 0 {
		t.Error("expected R12 violation for service import in CLI, got none")
	}
}

func TestR12_ViolationBiz(t *testing.T) {
	imports := []string{
		"aranea-agents/internal/biz/something",
	}
	rel := "internal/cli/cmd/bad.go"
	vs := r12CLINoBackendImport(rel, imports)
	if len(vs) == 0 {
		t.Error("expected R12 violation for biz import in internal/cli, got none")
	}
}
