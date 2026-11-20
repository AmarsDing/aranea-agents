package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListLocalDirs_SkipsHiddenAndSorts(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"b合同", "A资料", ".git", "$RECYCLE.BIN"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := listLocalDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != filepath.Clean(root) || got.Parent != filepath.Dir(filepath.Clean(root)) {
		t.Fatalf("path/parent = %q / %q", got.Path, got.Parent)
	}
	if len(got.Dirs) != 2 || got.Dirs[0].Name != "A资料" || got.Dirs[1].Name != "b合同" {
		t.Fatalf("dirs = %+v", got.Dirs)
	}
	if _, err := listLocalDirs("relative/path"); err == nil {
		t.Fatal("relative path must be rejected")
	}
	if roots, err := listLocalDirs(""); err != nil || len(roots.Dirs) == 0 || roots.Path != "" {
		t.Fatalf("roots = %+v, %v", roots, err)
	}
}

func TestCreateLocalDir(t *testing.T) {
	root := t.TempDir()
	abs, err := createLocalDir(root, "公司资料")
	if err != nil || abs != filepath.Join(root, "公司资料") {
		t.Fatalf("create = %q, %v", abs, err)
	}
	if _, err := createLocalDir(root, "公司资料"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second create err = %v, want ErrExist", err)
	}
	for _, bad := range []string{"", "..", `a\b`, "a/b", "a:b"} {
		if _, err := createLocalDir(root, bad); err == nil {
			t.Fatalf("name %q must be rejected", bad)
		}
	}
	if _, err := createLocalDir("relative", "x"); err == nil {
		t.Fatal("relative parent must be rejected")
	}
}

func TestLocalOpenExts_BlocksExecutables(t *testing.T) {
	for _, ext := range []string{".exe", ".bat", ".cmd", ".ps1", ".lnk", ".html", ".vbs", ".msi", ".js"} {
		if localOpenExts[ext] {
			t.Fatalf("%s must not be openable", ext)
		}
	}
	for _, ext := range []string{".docx", ".xlsx", ".pdf", ".md", ".png"} {
		if !localOpenExts[ext] {
			t.Fatalf("%s should be openable", ext)
		}
	}
}
