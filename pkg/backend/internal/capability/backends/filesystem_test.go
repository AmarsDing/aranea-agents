package backends

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileToolUsesWorkspaceSandbox(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", root)
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out, err := NewReadFileTool().Execute(nil, map[string]any{"path": "note.txt"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out["content"] != "hello" || out["path"] != "note.txt" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestReadFileToolRejectsOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", root)
	if _, _, err := ResolveWorkspacePath("../outside.txt"); err == nil {
		t.Fatalf("expected outside workspace path to fail")
	}
}

func TestEditFileToolRequiresUniqueMatch(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", root)
	path := filepath.Join(root, "dup.txt")
	if err := os.WriteFile(path, []byte("x\nx\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := NewEditFileTool().Execute(nil, map[string]any{
		"path":       "dup.txt",
		"old_string": "x",
		"new_string": "y",
	})
	if err == nil {
		t.Fatalf("expected ambiguous edit to fail")
	}
}
