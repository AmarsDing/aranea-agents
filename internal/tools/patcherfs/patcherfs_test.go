package patcherfs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspace_ReadWriteList_RoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "biz"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "biz", "x.go"), []byte("package biz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := New(root, ModeReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ws.Read("internal/biz/x.go", 0)
	if err != nil || !strings.Contains(got, "package biz") {
		t.Fatalf("Read = %q err=%v", got, err)
	}
	listing, err := ws.List("internal/biz")
	if err != nil || !strings.Contains(listing, "x.go") {
		t.Fatalf("List = %q err=%v", listing, err)
	}
	if err := ws.Write("internal/biz/x.go", "package biz\n// patched\n"); err != nil {
		t.Fatal(err)
	}
	got, err = ws.Read("internal/biz/x.go", 0)
	if err != nil || !strings.Contains(got, "patched") {
		t.Fatalf("after write Read = %q err=%v", got, err)
	}
}

func TestWorkspace_ReadModeRejectsWrite(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root, ModeRead)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Write("a.txt", "x"); err == nil {
		t.Fatal("read-only workspace must reject Write")
	}
	if msg := ws.Exec(Request{Tool: ToolWrite, Path: "a.txt", Content: "x"}); !strings.Contains(msg, "not allowed") {
		t.Fatalf("Exec write = %q", msg)
	}
}

func TestWorkspace_BlocksTraversalAndAbsolute(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root, ModeReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Read("../secret", 1024); err == nil {
		t.Fatal(".. must be rejected")
	}
	if _, err := ws.Read(`C:\Windows\win.ini`, 1024); err == nil {
		t.Fatal("absolute path must be rejected")
	}
	if msg := ws.Exec(Request{Tool: ToolRead, Path: ".."}); !strings.Contains(msg, "error:") {
		t.Fatalf("Exec traversal = %q", msg)
	}
}

func TestWorkspace_WriteRejectsProtected(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root, ModeReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Write("go.mod", "module x\n"); err == nil {
		t.Fatal("go.mod must be protected")
	}
	if err := ws.Write("Makefile", "all:\n"); err == nil {
		t.Fatal("Makefile must be protected")
	}
}

func TestWorkspace_ReadRejectsBinary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "blob.bin"), []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := New(root, ModeRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Read("blob.bin", 1024); err == nil {
		t.Fatal("binary read must fail")
	}
}

func TestParseRequest(t *testing.T) {
	req, ok := ParseRequest(`{"tool":"patcher_fs_read","path":"a.go"}`)
	if !ok || req.Tool != ToolRead || req.Path != "a.go" {
		t.Fatalf("ParseRequest = %+v ok=%v", req, ok)
	}
	if _, ok := ParseRequest(`{"root_cause":"x","fix_strategy":"y","impact_scope":"local","confidence":0.5}`); ok {
		t.Fatal("diagnosis JSON must not parse as a tool call")
	}
	req, ok = ParseRequest("```json\n{\"tool\":\"patcher_fs_list\",\"path\":\".\"}\n```")
	if !ok || req.Tool != ToolList {
		t.Fatalf("fenced tool call = %+v ok=%v", req, ok)
	}
}

func TestWorkspace_GitDiffAndRestore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "si@test.local")
	runGit(t, root, "config", "user.name", "si-test")
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")

	ws, err := New(root, ModeReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Write("note.txt", "v2\n"); err != nil {
		t.Fatal(err)
	}
	diff, err := ws.Diff("")
	if err != nil || !strings.Contains(diff, "+v2") {
		t.Fatalf("Diff = %q err=%v", diff, err)
	}
	if err := ws.Restore(); err != nil {
		t.Fatal(err)
	}
	got, err := ws.Read("note.txt", 0)
	if err != nil || strings.Contains(got, "v2") {
		t.Fatalf("after Restore Read = %q err=%v", got, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %s (%v)", strings.Join(args, " "), out, err)
	}
}
