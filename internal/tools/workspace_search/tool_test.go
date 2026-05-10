package workspace_search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldSkipPath(t *testing.T) {
	if !ShouldSkipPath("foo/node_modules/bar") {
		t.Fatal("expected skip node_modules")
	}
	if ShouldSkipPath("internal/foo.go") {
		t.Fatal("unexpected skip")
	}
}

func TestSearchWalkDir_substring(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello world\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".git", "config"), []byte("skip\n"), 0o644)

	matches, truncated, err := searchWalkDir(t.Context(), root, root, "hello", "substring", "", 10, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("unexpected truncate")
	}
	if len(matches) != 1 {
		t.Fatalf("matches=%d", len(matches))
	}
	if matches[0].Snippet != "hello world" {
		t.Fatalf("snippet %q", matches[0].Snippet)
	}
}
