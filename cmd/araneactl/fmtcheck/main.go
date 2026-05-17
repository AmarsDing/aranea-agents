// Command araneactl fmtcheck reports Go source files that are not formatted
// according to gofmt. Exit code 0 = all formatted; 1 = unformatted files found.
//
// Only first-party code is scanned (internal/, cmd/, pkg/auth/, pkg/safego/).
// Generated Ent files are skipped.
//
// Usage:
//
//	go run ./cmd/araneactl/fmtcheck          # scan from repo root
//	go run ./cmd/araneactl/fmtcheck --root . # explicit root
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := flag.String("root", ".", "repository root directory")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fmtcheck: cannot resolve root: %v\n", err)
		os.Exit(2)
	}

	scanDirs := []string{
		"internal",
		"cmd",
		filepath.Join("pkg", "auth"),
		filepath.Join("pkg", "safego"),
	}

	var unformatted []string
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
			// Skip generated Ent files.
			rel := relPath(abs, path)
			if strings.Contains(rel, "internal/data/ent/") &&
				!strings.Contains(rel, "/schema/") &&
				!strings.Contains(rel, "/hook/") {
				return nil
			}
			if bad, err := isUnformatted(path); err != nil {
				fmt.Fprintf(os.Stderr, "fmtcheck: cannot read %s: %v\n", rel, err)
			} else if bad {
				unformatted = append(unformatted, rel)
			}
			return nil
		}); err != nil {
			fmt.Fprintf(os.Stderr, "fmtcheck: walk error: %v\n", err)
			os.Exit(2)
		}
	}

	if len(unformatted) == 0 {
		fmt.Println("fmtcheck: OK — all files formatted")
		os.Exit(0)
	}

	fmt.Printf("fmtcheck: FAIL — %d unformatted file(s) (run: gofmt -w .)\n", len(unformatted))
	for _, f := range unformatted {
		fmt.Printf("  %s\n", f)
	}
	os.Exit(1)
}

func relPath(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

func isUnformatted(path string) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted, err := format.Source(src)
	if err != nil {
		// Syntax error in the file; not our problem here.
		return false, nil
	}
	return !bytes.Equal(src, formatted), nil
}
