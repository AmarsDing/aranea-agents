package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/biz/skill"
)

func newTestFS(root string) skill.SkillFilesystem {
	return NewSkillFilesystem(func(_ context.Context) string { return root })
}

func TestSafeFilePath_EmptyRelPath(t *testing.T) {
	fs := newTestFS(t.TempDir())
	_, _, err := fs.SafeFilePath(t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for empty relPath")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSafeFilePath_WhitespaceOnlyRelPath(t *testing.T) {
	fs := newTestFS(t.TempDir())
	_, _, err := fs.SafeFilePath(t.TempDir(), "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only relPath")
	}
}

func TestSafeFilePath_DotDot(t *testing.T) {
	fs := newTestFS(t.TempDir())
	dir := t.TempDir()
	_, _, err := fs.SafeFilePath(dir, "..")
	if err == nil {
		t.Fatal("expected error for relPath with ..")
	}
}

func TestSafeFilePath_DotDotInMiddle(t *testing.T) {
	fs := newTestFS(t.TempDir())
	dir := t.TempDir()
	_, _, err := fs.SafeFilePath(dir, "subdir/../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal with ..")
	}
}

func TestSafeFilePath_AbsolutePrefix(t *testing.T) {
	fs := newTestFS(t.TempDir())
	dir := t.TempDir()
	_, _, err := fs.SafeFilePath(dir, "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for relPath starting with /")
	}
}

func TestSafeFilePath_ValidFileName(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(dir)
	root, absPath, err := fs.SafeFilePath(dir, "SKILL.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedRoot, _ := filepath.Abs(dir)
	if root != expectedRoot {
		t.Fatalf("root = %q, want %q", root, expectedRoot)
	}
	expectedPath := filepath.Join(expectedRoot, "SKILL.md")
	if absPath != expectedPath {
		t.Fatalf("absPath = %q, want %q", absPath, expectedPath)
	}
}

func TestSafeFilePath_NestedPath(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(dir)
	root, absPath, err := fs.SafeFilePath(dir, "subdir/file.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedRoot, _ := filepath.Abs(dir)
	if root != expectedRoot {
		t.Fatalf("root = %q, want %q", root, expectedRoot)
	}
	expectedPath := filepath.Join(expectedRoot, "subdir", "file.md")
	if absPath != expectedPath {
		t.Fatalf("absPath = %q, want %q", absPath, expectedPath)
	}
}

func TestSafeFilePath_BackslashTraversal(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(dir)
	_, _, err := fs.SafeFilePath(dir, `subdir\..\..\etc\passwd`)
	if err == nil {
		t.Fatal("expected error for backslash path traversal")
	}
}

func TestSafeFilePath_DotDotAtEnd(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(dir)
	_, _, err := fs.SafeFilePath(dir, "subdir/..")
	if err == nil {
		t.Fatal("expected error for .. at end of path")
	}
}

func TestSafeFilePath_SameDir(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(dir)
	root, absPath, err := fs.SafeFilePath(dir, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedRoot, _ := filepath.Abs(dir)
	if root != expectedRoot {
		t.Fatalf("root = %q, want %q", root, expectedRoot)
	}
	if absPath != expectedRoot {
		t.Fatalf("absPath = %q, want %q", absPath, expectedRoot)
	}
}

func TestCreateSkillDir(t *testing.T) {
	root := t.TempDir()
	fs := newTestFS(root)

	dir, err := fs.CreateSkillDir("my-skill", "# My Skill\nHello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedDir := filepath.Join(root, "my-skill")
	if dir != expectedDir {
		t.Fatalf("dir = %q, want %q", dir, expectedDir)
	}

	st, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !st.IsDir() {
		t.Fatal("expected directory")
	}

	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if string(data) != "# My Skill\nHello" {
		t.Fatalf("SKILL.md content = %q, want %q", string(data), "# My Skill\nHello")
	}
}

func TestCreateSkillDir_ExistingDir(t *testing.T) {
	root := t.TempDir()
	fs := newTestFS(root)

	existing := filepath.Join(root, "existing-skill")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dir, err := fs.CreateSkillDir("existing-skill", "body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != existing {
		t.Fatalf("dir = %q, want %q", dir, existing)
	}

	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if string(data) != "body" {
		t.Fatalf("SKILL.md content = %q, want %q", string(data), "body")
	}
}

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("skill"), 0o644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("key: val"), 0o644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "subdir", "helper.py"), []byte("print('hi')"), 0o644)

	fs := newTestFS(t.TempDir())
	entries, err := fs.ListFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	byPath := map[string]skill.SkillFileEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}

	if e, ok := byPath["SKILL.md"]; !ok {
		t.Fatal("missing SKILL.md")
	} else {
		if e.Name != "SKILL.md" {
			t.Fatalf("Name = %q, want %q", e.Name, "SKILL.md")
		}
		if e.Language != "markdown" {
			t.Fatalf("Language = %q, want %q", e.Language, "markdown")
		}
	}

	if e, ok := byPath["config.yaml"]; !ok {
		t.Fatal("missing config.yaml")
	} else {
		if e.Language != "yaml" {
			t.Fatalf("Language = %q, want %q", e.Language, "yaml")
		}
	}

	if e, ok := byPath["subdir/helper.py"]; !ok {
		t.Fatal("missing subdir/helper.py")
	} else {
		if e.Language != "python" {
			t.Fatalf("Language = %q, want %q", e.Language, "python")
		}
		if e.Name != "helper.py" {
			t.Fatalf("Name = %q, want %q", e.Name, "helper.py")
		}
	}
}

func TestListFiles_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "emptydir"), 0o755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644)

	fs := newTestFS(t.TempDir())
	entries, err := fs.ListFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (directories should be skipped)", len(entries))
	}
	if entries[0].Path != "file.txt" {
		t.Fatalf("Path = %q, want %q", entries[0].Path, "file.txt")
	}
}

func TestListFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(t.TempDir())
	entries, err := fs.ListFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
}

func TestListFiles_RelativePathsUseForwardSlash(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "deep", "nested"), 0o755)
	os.WriteFile(filepath.Join(dir, "deep", "nested", "file.js"), []byte("1"), 0o644)

	fs := newTestFS(t.TempDir())
	entries, err := fs.ListFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if strings.Contains(entries[0].Path, "\\") {
		t.Fatalf("Path contains backslash: %q", entries[0].Path)
	}
	if entries[0].Path != "deep/nested/file.js" {
		t.Fatalf("Path = %q, want %q", entries[0].Path, "deep/nested/file.js")
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("hello world"), 0o644)

	fs := newTestFS(t.TempDir())
	content, err := fs.ReadFile(dir, "notes.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content.Content != "hello world" {
		t.Fatalf("Content = %q, want %q", content.Content, "hello world")
	}
	if content.Path != "notes.md" {
		t.Fatalf("Path = %q, want %q", content.Path, "notes.md")
	}
	if content.Language != "markdown" {
		t.Fatalf("Language = %q, want %q", content.Language, "markdown")
	}
}

func TestReadFile_NestedPath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "app.py"), []byte("print(1)"), 0o644)

	fs := newTestFS(t.TempDir())
	content, err := fs.ReadFile(dir, "sub/app.py")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content.Content != "print(1)" {
		t.Fatalf("Content = %q, want %q", content.Content, "print(1)")
	}
	if content.Path != "sub/app.py" {
		t.Fatalf("Path = %q, want %q", content.Path, "sub/app.py")
	}
	if content.Language != "python" {
		t.Fatalf("Language = %q, want %q", content.Language, "python")
	}
}

func TestReadFile_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	fs := newTestFS(t.TempDir())
	_, err := fs.ReadFile(dir, "subdir")
	if err == nil {
		t.Fatal("expected error when reading a directory")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadFile_RejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "big.bin")
	f, err := os.Create(bigFile)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.Write(make([]byte, 2*1024*1024+1)); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	fs := newTestFS(t.TempDir())
	_, err = fs.ReadFile(dir, "big.bin")
	if err == nil {
		t.Fatal("expected error for file > 2MB")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadFile_RejectsUnsafePath(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(t.TempDir())
	_, err := fs.ReadFile(dir, "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestReadFile_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(t.TempDir())
	_, err := fs.ReadFile(dir, "nope.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadFile_Exactly2MB(t *testing.T) {
	dir := t.TempDir()
	exactFile := filepath.Join(dir, "exact.bin")
	f, err := os.Create(exactFile)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.Write(make([]byte, 2*1024*1024)); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	fs := newTestFS(t.TempDir())
	_, err = fs.ReadFile(dir, "exact.bin")
	if err != nil {
		t.Fatalf("file exactly 2MB should be readable, got: %v", err)
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(t.TempDir())

	err := fs.WriteFile(dir, "new.txt", "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("content = %q, want %q", string(data), "content")
	}
}

func TestWriteFile_NestedPath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	fs := newTestFS(t.TempDir())

	err := fs.WriteFile(dir, "sub/nested.ts", "let x = 1;")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sub", "nested.ts"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "let x = 1;" {
		t.Fatalf("content = %q, want %q", string(data), "let x = 1;")
	}
}

func TestWriteFile_RejectsUnsafePath(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(t.TempDir())

	err := fs.WriteFile(dir, "../../../tmp/evil", "hack")
	if err == nil {
		t.Fatal("expected error for path traversal in WriteFile")
	}
}

func TestWriteFile_Overwrite(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("old"), 0o644)
	fs := newTestFS(t.TempDir())

	err := fs.WriteFile(dir, "file.txt", "new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "file.txt"))
	if string(data) != "new" {
		t.Fatalf("content = %q, want %q", string(data), "new")
	}
}

func TestDeleteFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "deleteme.txt")
	os.WriteFile(target, []byte("bye"), 0o644)

	fs := newTestFS(t.TempDir())
	err := fs.DeleteFile(dir, "deleteme.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("file should be deleted")
	}
}

func TestDeleteFile_RejectsUnsafePath(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(t.TempDir())

	err := fs.DeleteFile(dir, "../../important")
	if err == nil {
		t.Fatal("expected error for path traversal in DeleteFile")
	}
}

func TestDeleteFile_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(t.TempDir())

	err := fs.DeleteFile(dir, "ghost.txt")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent file")
	}
}

func TestRootAccessible_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(dir)
	if !fs.RootAccessible(context.Background()) {
		t.Fatal("expected RootAccessible=true for existing directory")
	}
}

func TestRootAccessible_NonexistentDir(t *testing.T) {
	fs := newTestFS(filepath.Join(t.TempDir(), "nope"))
	if fs.RootAccessible(context.Background()) {
		t.Fatal("expected RootAccessible=false for nonexistent directory")
	}
}

func TestRootAccessible_IsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "afile")
	os.WriteFile(filePath, []byte("x"), 0o644)

	fs := newTestFS(filePath)
	if fs.RootAccessible(context.Background()) {
		t.Fatal("expected RootAccessible=false when root is a file, not a directory")
	}
}

func TestDirExists_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	fs := newTestFS(t.TempDir())
	if !fs.DirExists(dir) {
		t.Fatal("expected DirExists=true for existing directory")
	}
}

func TestDirExists_NonexistentDir(t *testing.T) {
	fs := newTestFS(t.TempDir())
	if fs.DirExists(filepath.Join(t.TempDir(), "ghost")) {
		t.Fatal("expected DirExists=false for nonexistent directory")
	}
}

func TestDirExists_IsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "regular.txt")
	os.WriteFile(filePath, []byte("x"), 0o644)

	fs := newTestFS(t.TempDir())
	if fs.DirExists(filePath) {
		t.Fatal("expected DirExists=false when path is a file")
	}
}

func TestLanguageForPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"readme.md", "markdown"},
		{"doc.markdown", "markdown"},
		{"app.js", "javascript"},
		{"module.mjs", "javascript"},
		{"common.cjs", "javascript"},
		{"main.ts", "typescript"},
		{"script.py", "python"},
		{"data.json", "json"},
		{"conf.yaml", "yaml"},
		{"conf.yml", "yaml"},
		{"run.sh", "shell"},
		{"Makefile", "text"},
		{"image.png", "text"},
		{"noext", "text"},
		{"UPPER.MD", "markdown"},
	}
	for _, tc := range cases {
		got := languageForPath(tc.path)
		if got != tc.want {
			t.Errorf("languageForPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestPathBase(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"SKILL.md", "SKILL.md"},
		{"subdir/file.md", "file.md"},
		{"a/b/c/d.js", "d.js"},
		{"/leading/slash.txt", "slash.txt"},
		{"trailing/", "trailing"},
		{"", ""},
		{".", ""},
		{"/", ""},
	}
	for _, tc := range cases {
		got := pathBase(tc.input)
		if got != tc.want {
			t.Errorf("pathBase(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNewSkillFilesystem_NilResolveFn(t *testing.T) {
	fs := NewSkillFilesystem(nil)
	if fs == nil {
		t.Fatal("expected non-nil filesystem")
	}
}

func TestResolveRoot(t *testing.T) {
	expected := "/test/root"
	fs := newTestFS(expected)
	got := fs.ResolveRoot(context.Background())
	if got != expected {
		t.Fatalf("ResolveRoot() = %q, want %q", got, expected)
	}
}
