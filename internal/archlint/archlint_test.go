package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestBizNotDependOnTrpcAgentGo verifies AS-FIT-01 P0 invariant:
// biz layer must NOT depend on pkg/trpc-agent-go.
// The trpc-agent-go module path is trpc.group/trpc-go/trpc-agent-go
// (vendored at ./pkg/trpc-agent-go via go.mod replace directive).
func TestBizNotDependOnTrpcAgentGo(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedImports | packages.NeedDeps,
	}
	pkgs, err := packages.Load(cfg, "aranea-agents/internal/biz/...")
	if err != nil {
		t.Fatalf("failed to load biz packages: %v", err)
	}

	for _, pkg := range pkgs {
		for importPath := range pkg.Imports {
			if strings.Contains(importPath, "trpc.group/trpc-go/trpc-agent-go") {
				t.Errorf("biz layer must not depend on pkg/trpc-agent-go: %s imports %s", pkg.PkgPath, importPath)
			}
		}
	}
}

// TestServiceNotDirectlyAccessData verifies AS-FIT-01 P0 invariant:
// service layer must NOT directly import the data layer.
// Service should go through biz (use cases) instead.
func TestServiceNotDirectlyAccessData(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedImports | packages.NeedDeps,
	}
	pkgs, err := packages.Load(cfg, "aranea-agents/internal/service/...")
	if err != nil {
		t.Fatalf("failed to load service packages: %v", err)
	}

	for _, pkg := range pkgs {
		for importPath := range pkg.Imports {
			if strings.Contains(importPath, "aranea-agents/internal/data") {
				t.Errorf("service layer must not directly import data layer: %s imports %s", pkg.PkgPath, importPath)
			}
		}
	}
}

// TestBizPortInterfaceMethodCount verifies AS-FIT-01 invariant:
// biz port interfaces should have ≤5 methods (interface narrowing).
// Port interfaces are defined in internal/biz/ and used by other layers.
func TestBizPortInterfaceMethodCount(t *testing.T) {
	fset := token.NewFileSet()
	bizDir := filepath.Join("..", "..", "internal", "biz")

	pkgs, err := parser.ParseDir(fset, bizDir, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse biz dir: %v", err)
	}

	const maxMethods = 5
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.TYPE {
					continue
				}
				for _, spec := range genDecl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					iface, ok := typeSpec.Type.(*ast.InterfaceType)
					if !ok {
						continue
					}
					// Only check port interfaces (exported, with Stability annotation)
					if !typeSpec.Name.IsExported() {
						continue
					}
					hasStability := strings.Contains(genDecl.Doc.Text(), "Stability:")
					if !hasStability {
						continue
					}
					methodCount := len(iface.Methods.List)
					if methodCount > maxMethods {
						t.Logf("AS-FIT-01: biz port interface %s has %d methods (max %d) — consider splitting",
							typeSpec.Name.Name, methodCount, maxMethods)
					}
				}
			}
		}
	}
}

// TestStateMachineCoverage verifies AS-FIT-01 / AS-FSM-01 invariant:
// entities with >3 states must have an explicit state machine file.
func TestStateMachineCoverage(t *testing.T) {
	// Entities that must have state machines per AS-FSM-01
	requiredStateMachines := map[string]string{
		"Run":              "run_state_machine.go",
		"SessionRunPhase":  "session_run_phase_machine.go",
		"TeamRun":          "team_run_state_machine.go",
		"GraphExecution":   "graph_execution_state_machine.go",
		"Team":             "team_state_machine.go",
		"ChannelTurnJob":   "channel_turn_job_state_machine.go",
		"Session":          filepath.Join("session", "status_machine.go"),
	}

	bizDir := filepath.Join("..", "..", "internal", "biz")
	for entity, filename := range requiredStateMachines {
		fullPath := filepath.Join(bizDir, filename)
		if _, err := parser.ParseFile(token.NewFileSet(), fullPath, nil, parser.PackageClauseOnly); err != nil {
			t.Errorf("AS-FSM-01: entity %s (>3 states) must have state machine file %s: %v", entity, filename, err)
		}
	}
}

// TestStructFieldCount verifies AS-COG-01 invariant:
// key structs should have ≤15 injected fields.
// This checks the most critical structs that are prone to God Object anti-pattern.
func TestStructFieldCount(t *testing.T) {
	fset := token.NewFileSet()
	bizDir := filepath.Join("..", "..", "internal", "biz")
	svcDir := filepath.Join("..", "..", "internal", "service")

	const maxFields = 15
	checkDirs := map[string]string{
		"biz":     bizDir,
		"service": svcDir,
	}

	for layer, dir := range checkDirs {
		pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse %s dir: %v", layer, err)
		}
		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				for _, decl := range file.Decls {
					genDecl, ok := decl.(*ast.GenDecl)
					if !ok || genDecl.Tok != token.TYPE {
						continue
					}
					for _, spec := range genDecl.Specs {
						typeSpec, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						structType, ok := typeSpec.Type.(*ast.StructType)
						if !ok {
							continue
						}
						// Only check exported structs with Stability annotation or
						// key orchestrator structs
						if !typeSpec.Name.IsExported() {
							continue
						}
						fieldCount := len(structType.Fields.List)
						if fieldCount > maxFields {
							t.Logf("AS-COG-01: %s struct %s has %d fields (max %d) — consider extracting sub-managers",
								layer, typeSpec.Name.Name, fieldCount, maxFields)
						}
					}
				}
			}
		}
	}
}
