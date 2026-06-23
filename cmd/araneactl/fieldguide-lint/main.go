// Command fieldguide-lint validates that the TypeScript FieldGuide registry
// (web/src/features/agents/fieldGuides.ts) stays in sync with the Go registry
// (internal/biz/field_guides.go).
//
// The check is intentionally simple: it extracts scope strings from each file
// and reports any that exist in one but not the other.
//
// Usage:
//
//	go run ./cmd/araneactl/fieldguide-lint          # scan from repo root
//	go run ./cmd/araneactl/fieldguide-lint --root . # explicit root
//
// Exit code 0 = in sync; 1 = drift detected; 2 = tool error.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var tsScopeLineRE = regexp.MustCompile(`^\s*\|\s*'([^']+)'\s*,?\s*;?\s*$`)

func main() {
	root := flag.String("root", ".", "repository root directory")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fieldguide-lint: cannot resolve root: %v\n", err)
		os.Exit(2)
	}

	goScopes, err := extractGoScopes(filepath.Join(abs, "internal", "biz", "field_guides.go"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fieldguide-lint: reading Go registry: %v\n", err)
		os.Exit(2)
	}

	tsScopes, err := extractTSScopes(filepath.Join(abs, "web", "src", "features", "agents", "fieldGuides.ts"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fieldguide-lint: reading TS registry: %v\n", err)
		os.Exit(2)
	}

	// Filter out spec_extract — it's a backend-only scope used by CLI import; no TS UI needed.
	const backendOnlyScope = "spec_extract"

	var drifts []string
	for scope := range goScopes {
		if scope == backendOnlyScope {
			continue
		}
		if !tsScopes[scope] {
			drifts = append(drifts, fmt.Sprintf("  Go has scope %q but TypeScript does not", scope))
		}
	}
	for scope := range tsScopes {
		if !goScopes[scope] {
			drifts = append(drifts, fmt.Sprintf("  TypeScript has scope %q but Go does not", scope))
		}
	}

	if len(drifts) == 0 {
		fmt.Printf("fieldguide-lint: OK — %d scopes in sync\n", len(goScopes)-1) // -1 for spec_extract
		os.Exit(0)
	}

	fmt.Printf("fieldguide-lint: FAIL — %d scope drift(s):\n", len(drifts))
	for _, d := range drifts {
		fmt.Println(d)
	}
	fmt.Println()
	fmt.Println("Fix: update web/src/features/agents/fieldGuides.ts to match internal/biz/field_guides.go (or vice versa).")
	os.Exit(1)
}

// extractGoScopes scans field_guides.go for FieldScope string constant values.
// Looks for lines of the form: ScopeXxx FieldScope = "some.scope"
func extractGoScopes(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scopes := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		// Match: ScopeCategoryIndustry   FieldScope = "category.industry"
		if !strings.Contains(line, "FieldScope") || !strings.Contains(line, `"`) {
			continue
		}
		val := extractQuoted(line)
		if val != "" {
			scopes[val] = true
		}
	}
	return scopes, sc.Err()
}

// extractTSScopes scans fieldGuides.ts for FieldScope type union values.
// Looks for lines of the form: | 'some.scope'
func extractTSScopes(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scopes := map[string]bool{}
	sc := bufio.NewScanner(f)
	inFieldScopeType := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.Contains(line, "export type FieldScope") {
			inFieldScopeType = true
			continue
		}
		if !inFieldScopeType {
			continue
		}
		if strings.HasSuffix(line, ";") {
			inFieldScopeType = false
		}
		if m := tsScopeLineRE.FindStringSubmatch(line); len(m) == 2 {
			scopes[m[1]] = true
		}
	}
	return scopes, sc.Err()
}

// extractQuoted returns the first double-quoted string from s.
func extractQuoted(s string) string {
	start := strings.Index(s, `"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start+1:], `"`)
	if end < 0 {
		return ""
	}
	return s[start+1 : start+1+end]
}
