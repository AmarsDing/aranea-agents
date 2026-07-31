package patcherfs

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// initWorktreeFixture creates a temp git repo acting as the patcher worktree.
func initWorktreeFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	root := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() { println(\"hi\") }\n"), 0o644); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "initial"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return root
}

func callTool(t *testing.T, tool trpctool.CallableTool, args map[string]any) (any, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return tool.Call(context.Background(), raw)
}

// asCallable narrows a registered tool to CallableTool for direct invocation.
func asCallable(t *testing.T, tool trpctool.Tool) trpctool.CallableTool {
	t.Helper()
	c, ok := tool.(trpctool.CallableTool)
	if !ok {
		t.Fatalf("tool %s is not CallableTool", tool.Declaration().Name)
	}
	return c
}

// ── registration ─────────────────────────────────────────────────────────────

func TestRegisterAll_ToolNames(t *testing.T) {
	root := initWorktreeFixture(t)
	tools := RegisterAll(root, loggateway.NewNoop())
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Declaration().Name] = true
	}
	for _, want := range []string{"patcher_fs_read", "patcher_fs_write", "patcher_git_diff"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestRegisterAll_EmptyRootReturnsNil(t *testing.T) {
	if tools := RegisterAll("", loggateway.NewNoop()); tools != nil {
		t.Errorf("empty worktree root should yield nil tools, got %d", len(tools))
	}
}

// ── fs_read ──────────────────────────────────────────────────────────────────

func TestFsRead_ReadsCommittedFile(t *testing.T) {
	root := initWorktreeFixture(t)
	tools := RegisterAll(root, loggateway.NewNoop())
	read := asCallable(t, tools[0])

	out, err := callTool(t, read, map[string]any{"path": "main.go"})
	if err != nil {
		t.Fatalf("fs_read: %v", err)
	}
	res := out.(fsReadOutput)
	if !strings.Contains(res.Content, "package main") {
		t.Errorf("unexpected content: %q", res.Content)
	}
	if res.Truncated {
		t.Error("small file should not truncate")
	}
}

func TestFsRead_RejectsEscapeAndGitDir(t *testing.T) {
	root := initWorktreeFixture(t)
	read := asCallable(t, RegisterAll(root, loggateway.NewNoop())[0])

	for _, p := range []string{
		"../outside.go",
		"..",
		".git/config",
		"/etc/passwd",
		`\windows\system32\drivers\etc\hosts`,
	} {
		if _, err := callTool(t, read, map[string]any{"path": p}); err == nil {
			t.Errorf("path %q must be rejected", p)
		}
	}
}

func TestFsRead_TruncatesLargeFile(t *testing.T) {
	root := initWorktreeFixture(t)
	big := strings.Repeat("a", 4096)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	read := asCallable(t, RegisterAll(root, loggateway.NewNoop())[0])

	out, err := callTool(t, read, map[string]any{"path": "big.txt", "max_bytes": 1024})
	if err != nil {
		t.Fatalf("fs_read: %v", err)
	}
	res := out.(fsReadOutput)
	if !res.Truncated {
		t.Error("expected truncated=true")
	}
	if len(res.Content) != 1024 {
		t.Errorf("content len = %d, want 1024", len(res.Content))
	}
}

func TestFsRead_RejectsBinary(t *testing.T) {
	root := initWorktreeFixture(t)
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0x00, 0x01, 0x02, 0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	read := asCallable(t, RegisterAll(root, loggateway.NewNoop())[0])
	if _, err := callTool(t, read, map[string]any{"path": "bin.dat"}); err == nil {
		t.Error("binary file must be rejected")
	}
}

func TestFsRead_SymlinkEscapeRejected(t *testing.T) {
	root := initWorktreeFixture(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not permitted on this platform: %v", err)
	}
	read := asCallable(t, RegisterAll(root, loggateway.NewNoop())[0])
	if _, err := callTool(t, read, map[string]any{"path": "link.txt"}); err == nil {
		t.Error("symlink escaping the worktree must be rejected")
	}
}

// ── fs_write ─────────────────────────────────────────────────────────────────

func TestFsWrite_CreatesFileAndDirs(t *testing.T) {
	root := initWorktreeFixture(t)
	write := asCallable(t, RegisterAll(root, loggateway.NewNoop())[1])

	out, err := callTool(t, write, map[string]any{
		"path":    "pkg/newmod/mod.go",
		"content": "package newmod\n",
	})
	if err != nil {
		t.Fatalf("fs_write: %v", err)
	}
	res := out.(fsWriteOutput)
	if res.Bytes != len("package newmod\n") {
		t.Errorf("bytes = %d", res.Bytes)
	}
	got, err := os.ReadFile(filepath.Join(root, "pkg", "newmod", "mod.go"))
	if err != nil || string(got) != "package newmod\n" {
		t.Errorf("written content mismatch: %q, err=%v", got, err)
	}
}

func TestFsWrite_RejectsEscapeAndGitDir(t *testing.T) {
	root := initWorktreeFixture(t)
	write := asCallable(t, RegisterAll(root, loggateway.NewNoop())[1])

	for _, p := range []string{"../evil.go", ".git/hooks/evil", "/abs/path.go"} {
		_, err := callTool(t, write, map[string]any{"path": p, "content": "x"})
		if err == nil {
			t.Errorf("write path %q must be rejected", p)
		}
	}
}

// ── git_diff ─────────────────────────────────────────────────────────────────

func TestGitDiff_CapturesModifyAndNewFile(t *testing.T) {
	root := initWorktreeFixture(t)
	tools := RegisterAll(root, loggateway.NewNoop())
	write := asCallable(t, tools[1])
	diffTool := asCallable(t, tools[2])

	// Modify tracked file.
	if _, err := callTool(t, write, map[string]any{
		"path": "main.go", "content": "package main\n\nfunc main() { println(\"patched\") }\n",
	}); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	// Add new untracked file.
	if _, err := callTool(t, write, map[string]any{
		"path": "pkg/foo/foo.go", "content": "package foo\n",
	}); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}

	out, err := callTool(t, diffTool, map[string]any{})
	if err != nil {
		t.Fatalf("git_diff: %v", err)
	}
	res := out.(gitDiffOutput)
	if !strings.Contains(res.Diff, "patched") {
		t.Errorf("diff missing modification: %q", res.Diff)
	}
	if !strings.Contains(res.Diff, "pkg/foo/foo.go") {
		t.Errorf("diff missing new untracked file: %q", res.Diff)
	}
	if res.Stats.Files != 2 {
		t.Errorf("stats files = %d, want 2", res.Stats.Files)
	}
}

func TestGitDiff_EmptyWhenClean(t *testing.T) {
	root := initWorktreeFixture(t)
	diffTool := asCallable(t, RegisterAll(root, loggateway.NewNoop())[2])

	out, err := callTool(t, diffTool, map[string]any{})
	if err != nil {
		t.Fatalf("git_diff: %v", err)
	}
	res := out.(gitDiffOutput)
	if strings.TrimSpace(res.Diff) != "" {
		t.Errorf("clean worktree should produce empty diff, got %q", res.Diff)
	}
	if res.Stats.Files != 0 {
		t.Errorf("stats files = %d, want 0", res.Stats.Files)
	}
}
