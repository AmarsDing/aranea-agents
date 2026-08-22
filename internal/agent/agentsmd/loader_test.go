package agentsmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsTrusted(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inside := filepath.Join(root, "pkg")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsTrusted(inside, []string{root}) {
		t.Fatal("path under trusted root must be trusted")
	}
	if IsTrusted(t.TempDir(), []string{root}) {
		t.Fatal("sibling temp dir must be untrusted")
	}
	if IsTrusted(inside, nil) {
		t.Fatal("empty trusted list must skip")
	}
}

func TestLoad_NestedOverrideAndBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("root rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "svc")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "AGENTS.override.md"), []byte("nested override"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(nested, []string{root}, DefaultMaxBytes)
	if !strings.Contains(got.Text, "root rules") || !strings.Contains(got.Text, "nested override") {
		t.Fatalf("chain missing layers: %q", got.Text)
	}
	if idxRoot := strings.Index(got.Text, "root rules"); idxRoot > strings.Index(got.Text, "nested override") {
		t.Fatal("root doc must precede nested override")
	}
	tiny := Load(nested, []string{root}, 8)
	if !tiny.Truncated {
		t.Fatal("over-budget load must set Truncated")
	}
}

func TestLoad_UntrustedSkipped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(root, []string{t.TempDir()}, DefaultMaxBytes)
	if got.Text != "" {
		t.Fatalf("untrusted root must not load, got %q", got.Text)
	}
}

func TestLoad_ClaudeFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("claude rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(root, []string{root}, DefaultMaxBytes)
	if !strings.Contains(got.Text, "claude rules") {
		t.Fatalf("expected CLAUDE.md fallback, got %q", got.Text)
	}
	block := FormatBlock(got)
	if !strings.Contains(block, "<project_agents_md>") {
		t.Fatalf("format block: %q", block)
	}
}
