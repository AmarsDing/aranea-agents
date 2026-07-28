package archlint

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
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

// TestFileLineCount enforces the AS-COG-01 invariant "file lines ≤ 500" as a
// ratchet gate (P2-Y2):
//
//   - A .go file NOT in file_line_baseline.txt exceeding 500 lines → FAIL
//     (new debt is rejected).
//   - A baseline-listed file that grew beyond its recorded count → FAIL
//     (existing debt may only shrink).
//   - A baseline-listed file now ≤500 lines → logged, entry should be removed.
//   - A baseline entry whose file no longer exists → logged, entry should be
//     removed.
//
// Scope: internal/ and cmd/, excluding generated code (internal/data/ent/,
// *.pb.go, wire_gen.go) and the vendored pkg/trpc-agent-go tree.
//
// Regenerate the baseline after a sanctioned split/refactor (count EVERY line
// including blanks — do NOT use `Measure-Object -Line`, which skips blanks):
//
//	$files = Get-ChildItem -Path internal,cmd -Recurse -Filter *.go | ? {
//	  $_.FullName -notmatch '\\ent\\' -and $_.Name -notmatch '\.pb\.go$' -and
//	  $_.Name -ne 'wire_gen.go' -and $_.FullName -notmatch 'pkg\\trpc-agent-go' }
//	foreach ($f in $files) { $n = [System.IO.File]::ReadAllLines($f.FullName).Count
//	  if ($n -gt 500) { "$($f.FullName -replace [regex]::Escape((Get-Location).Path+'\'),'' -replace '\\','/') $n" } }
func TestFileLineCount(t *testing.T) {
	const maxLines = 500
	repoRoot := filepath.Join("..", "..")

	baseline, err := loadLineCountBaseline(filepath.Join(repoRoot, "internal", "archlint", "file_line_baseline.txt"))
	if err != nil {
		t.Fatalf("failed to load file-line baseline: %v", err)
	}

	seen := make(map[string]bool, len(baseline))
	for _, root := range []string{"internal", "cmd"} {
		walkRoot := filepath.Join(repoRoot, root)
		err := filepath.WalkDir(walkRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Skip generated ent tree and vendored trpc-agent-go.
				rel := filepath.ToSlash(mustRel(t, repoRoot, path))
				if rel == "internal/data/ent" || strings.HasPrefix(rel, "pkg/trpc-agent-go") {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".pb.go") || name == "wire_gen.go" {
				return nil
			}
			rel := filepath.ToSlash(mustRel(t, repoRoot, path))
			lines, lerr := countFileLines(path)
			if lerr != nil {
				t.Errorf("count lines %s: %v", rel, lerr)
				return nil
			}
			base, listed := baseline[rel]
			if listed {
				seen[rel] = true
				switch {
				case lines <= maxLines:
					t.Logf("AS-COG-01: %s is now %d lines (<= %d) — remove it from file_line_baseline.txt", rel, lines, maxLines)
				case lines > base:
					t.Errorf("AS-COG-01: %s grew to %d lines (baseline %d) — file debt may only shrink; split the file or lower the baseline entry after a sanctioned refactor", rel, lines, base)
				}
				return nil
			}
			if lines > maxLines {
				t.Errorf("AS-COG-01: %s has %d lines (max %d) and is not in file_line_baseline.txt — split the file before adding new code to it", rel, lines, maxLines)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	for rel := range baseline {
		if !seen[rel] {
			if _, statErr := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel))); os.IsNotExist(statErr) {
				t.Logf("AS-COG-01: baseline entry %s no longer exists — remove it from file_line_baseline.txt", rel)
			}
		}
	}
}

// loadLineCountBaseline parses file_line_baseline.txt into path → line count.
// Lines starting with '#' and blank lines are ignored.
func loadLineCountBaseline(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]int)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid baseline line in %s: %q", path, line)
		}
		n, cerr := strconv.Atoi(fields[1])
		if cerr != nil {
			return nil, fmt.Errorf("invalid baseline line in %s: %q", path, line)
		}
		out[fields[0]] = n
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// countFileLines returns the number of lines in the file (matching
// (Get-Content | Measure-Object -Line) semantics: newline-terminated records).
func countFileLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}

func mustRel(t *testing.T, base, target string) string {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("rel %s → %s: %v", base, target, err)
	}
	return rel
}

// TestStateMachineCoverage verifies AS-FIT-01 / AS-FSM-01 invariant:
// entities with >3 states must have an explicit state machine file.
func TestStateMachineCoverage(t *testing.T) {
	// Entities that must have state machines per AS-FSM-01
	requiredStateMachines := map[string]string{
		"Run":             "run_state_machine.go",
		"SessionRunPhase": "session_run_phase_machine.go",
		"TeamRun":         "team_run_state_machine.go",
		"GraphExecution":  "graph_execution_state_machine.go",
		"Team":            "team_state_machine.go",
		"ChannelTurnJob":  "channel_turn_job_state_machine.go",
		"Session":         filepath.Join("session", "status_machine.go"),
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
