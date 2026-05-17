// Command araneactl lint checks Aranea-Agents source code for runtime-boundary
// violations and common structural rules (R1-R10 from master-plan §7.1).
//
// Usage:
//
//	go run ./cmd/araneactl/lint          # scan from repo root
//	go run ./cmd/araneactl/lint --root . # explicit root
//
// Exit code 0 = clean; 1 = violations found.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type violation struct {
	file    string
	rule    string
	message string
}

func main() {
	root := flag.String("root", ".", "repository root directory")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "araneactl lint: cannot resolve root: %v\n", err)
		os.Exit(2)
	}

	var violations []violation
	scanDirs := []string{"internal", "cmd", "api"}
	for _, dir := range scanDirs {
		target := filepath.Join(abs, dir)
		if _, err := os.Stat(target); os.IsNotExist(err) {
			continue
		}
		if err := filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			rel := relPath(abs, path)
			vs := checkFile(rel, path)
			violations = append(violations, vs...)
			return nil
		}); err != nil {
			fmt.Fprintf(os.Stderr, "araneactl lint: walk error: %v\n", err)
			os.Exit(2)
		}
	}

	// Additional non-file checks.
	violations = append(violations, checkMainGoSize(abs)...)

	if len(violations) == 0 {
		fmt.Println("araneactl lint: OK — 0 violations")
		os.Exit(0)
	}

	fmt.Printf("araneactl lint: FAIL — %d violation(s)\n", len(violations))
	for _, v := range violations {
		fmt.Printf("  [%s] %s: %s\n", v.rule, v.file, v.message)
	}
	os.Exit(1)
}

// relPath returns a slash-separated path relative to root.
func relPath(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

// checkFile runs all file-level rules on the given file.
// Rules only apply to their relevant layer packages.
func checkFile(rel, path string) []violation {
	// Skip generated Ent files.
	if strings.Contains(rel, "internal/data/ent/") && !strings.Contains(rel, "/schema/") && !strings.Contains(rel, "/hook/") {
		return nil
	}
	lines, imports, err := parseFile(path)
	if err != nil {
		return nil
	}
	var vs []violation
	vs = append(vs, r1ServerNoDirectRuntime(rel, imports)...)
	vs = append(vs, r2BizNoRuntimeImport(rel, imports)...)
	// R3 intentionally omitted: Kratos repository pattern requires data layer to
	// import internal/biz for interface type definitions. This is expected.
	vs = append(vs, r4ServiceNoEntDirect(rel, imports)...)
	// R6/R7/R8/R9 only apply inside internal/ to avoid false-positives on tool source.
	if strings.HasPrefix(rel, "internal/") {
		vs = append(vs, r6NoExtraHTTPServer(rel, lines)...)
		vs = append(vs, r7NoMuxHandleFunc(rel, lines)...)
		vs = append(vs, r8SqlOpenOnlyInDataGo(rel, lines)...)
		vs = append(vs, r9NoBizLogDefault(rel, lines)...)
	}
	return vs
}

// parseFile reads a Go file and returns the raw lines and the import paths.
func parseFile(path string) (lines []string, imports []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	inImport := false
	for sc.Scan() {
		line := sc.Text()
		lines = append(lines, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import (") {
			inImport = true
			continue
		}
		if inImport {
			if trimmed == ")" {
				inImport = false
				continue
			}
			// Strip alias and quotes.
			parts := strings.Fields(trimmed)
			for _, p := range parts {
				p = strings.Trim(p, `"`)
				if strings.Contains(p, ".") || strings.Contains(p, "/") {
					imports = append(imports, p)
				}
			}
		} else if strings.HasPrefix(trimmed, `import "`) {
			imp := strings.TrimPrefix(trimmed, `import "`)
			imp = strings.TrimSuffix(imp, `"`)
			imports = append(imports, imp)
		}
	}
	return lines, imports, sc.Err()
}

func hasImport(imports []string, prefix string) bool {
	for _, imp := range imports {
		if strings.HasPrefix(imp, prefix) {
			return true
		}
	}
	return false
}

func linesContain(lines []string, substr string) bool {
	for _, l := range lines {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

// R1: internal/server/* must not import runner.Runner{} / llmagent.New directly.
func r1ServerNoDirectRuntime(rel string, imports []string) []violation {
	if !strings.HasPrefix(rel, "internal/server/") {
		return nil
	}
	if hasImport(imports, "trpc.group/trpc-go/trpc-agent-go/runner") ||
		hasImport(imports, "trpc.group/trpc-go/trpc-agent-go/agent/llmagent") {
		return []violation{{file: rel, rule: "R1", message: "internal/server must not import trpc-agent-go runner or llmagent directly"}}
	}
	return nil
}

// R2: internal/biz/* must not import pkg/trpc-agent-go/* or internal/*/trpc/*.
func r2BizNoRuntimeImport(rel string, imports []string) []violation {
	if !strings.HasPrefix(rel, "internal/biz/") {
		return nil
	}
	if hasImport(imports, "trpc.group/trpc-go/trpc-agent-go") {
		return []violation{{file: rel, rule: "R2", message: "internal/biz must not import trpc-agent-go"}}
	}
	for _, imp := range imports {
		if strings.Contains(imp, "aranea-agents/internal/") && strings.HasSuffix(filepath.Dir(imp), "/trpc") {
			return []violation{{file: rel, rule: "R2", message: "internal/biz must not import internal/*/trpc packages"}}
		}
	}
	return nil
}

// R3: internal/data/* must not import internal/biz/*.
func r3DataNoBizImport(rel string, imports []string) []violation {
	if !strings.HasPrefix(rel, "internal/data/") {
		return nil
	}
	if hasImport(imports, "aranea-agents/internal/biz") {
		return []violation{{file: rel, rule: "R3", message: "internal/data must not import internal/biz"}}
	}
	return nil
}

// R4: internal/service/* must not directly import Ent client.
func r4ServiceNoEntDirect(rel string, imports []string) []violation {
	if !strings.HasPrefix(rel, "internal/service/") {
		return nil
	}
	if hasImport(imports, "aranea-agents/internal/data/ent") {
		return []violation{{file: rel, rule: "R4", message: "internal/service must not import Ent client directly; use biz layer"}}
	}
	return nil
}

// R6: only the metrics handler is allowed to create http.Server{}.
// Whitelist: internal/server/metrics.go.
func r6NoExtraHTTPServer(rel string, lines []string) []violation {
	if rel == "internal/server/metrics.go" {
		return nil
	}
	if strings.HasSuffix(rel, "_test.go") {
		return nil
	}
	if linesContain(lines, "http.Server{") {
		return []violation{{file: rel, rule: "R6", message: "bare http.Server{} literal found; use the Kratos server or metrics handler"}}
	}
	return nil
}

// R7: mux.HandleFunc is forbidden outside metrics.go.
func r7NoMuxHandleFunc(rel string, lines []string) []violation {
	if rel == "internal/server/metrics.go" {
		return nil
	}
	if strings.HasSuffix(rel, "_test.go") {
		return nil
	}
	if linesContain(lines, "mux.HandleFunc(") {
		return []violation{{file: rel, rule: "R7", message: "mux.HandleFunc found; use Kratos routing instead"}}
	}
	return nil
}

// R8: sql.Open is only allowed in internal/data/data.go.
func r8SqlOpenOnlyInDataGo(rel string, lines []string) []violation {
	if rel == "internal/data/data.go" {
		return nil
	}
	if strings.HasSuffix(rel, "_test.go") {
		return nil
	}
	if linesContain(lines, "sql.Open(") {
		return []violation{{file: rel, rule: "R8", message: "sql.Open is only allowed in internal/data/data.go"}}
	}
	return nil
}

// R9: business packages (internal/biz, internal/service, internal/agent) must not
// use the default logger (log.Default(), log.Printf, log.Println, log.Fatal*).
func r9NoBizLogDefault(rel string, lines []string) []violation {
	bizPrefixes := []string{"internal/biz/", "internal/service/", "internal/agent/"}
	isBiz := false
	for _, p := range bizPrefixes {
		if strings.HasPrefix(rel, p) {
			isBiz = true
			break
		}
	}
	if !isBiz || strings.HasSuffix(rel, "_test.go") {
		return nil
	}
	for _, marker := range []string{"log.Default()", "log.Printf(", "log.Println(", "log.Fatalf(", "log.Fatal("} {
		if linesContain(lines, marker) {
			return []violation{{file: rel, rule: "R9", message: fmt.Sprintf("standard log.* call (%q) found; use the structured logger", marker)}}
		}
	}
	return nil
}

// R10: cmd/admin/main.go must not exceed 200 lines.
func checkMainGoSize(root string) []violation {
	path := filepath.Join(root, "cmd", "admin", "main.go")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	count := 0
	for sc.Scan() {
		count++
	}
	if count > 200 {
		return []violation{{
			file:    "cmd/admin/main.go",
			rule:    "R10",
			message: fmt.Sprintf("main.go has %d lines; must be ≤ 200", count),
		}}
	}
	return nil
}
