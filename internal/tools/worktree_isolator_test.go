package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// initTempGitRepo creates a temporary git repository with an initial commit.
// Returns the repo path and a cleanup function. Skips the test if git is not
// available on PATH.
func initTempGitRepo(t *testing.T) (string, func()) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available on PATH: %v", err)
	}
	tmpDir, err := os.MkdirTemp("", "worktree-iso-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	// git init + configure + initial commit
	steps := [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			cleanup()
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	// Create an initial file so HEAD exists.
	seed := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(seed, []byte("# test repo\n"), 0o644); err != nil {
		cleanup()
		t.Fatalf("WriteFile: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			cleanup()
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return tmpDir, cleanup
}

// currentHead returns the current HEAD commit hash of the repo at repoRoot.
func currentHead(t *testing.T, repoRoot string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// fileExistsInRepo checks whether a file exists at the repo root.
func fileExistsInRepo(t *testing.T, repoRoot, name string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(repoRoot, name))
	return err == nil
}

// TestWorktreeIsolator_CreatesAndMergesOnSuccess verifies that a successful
// handler invocation creates a worktree, commits changes, and fast-forward
// merges the branch back to the base branch.
func TestWorktreeIsolator_CreatesAndMergesOnSuccess(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()

	headBefore := currentHead(t, repoRoot)

	// Handler writes a new file into the worktree and commits it.
	handler := func(ctx context.Context, worktreeDir string, call ToolCall) ToolResult {
		newFile := filepath.Join(worktreeDir, "new.txt")
		if err := os.WriteFile(newFile, []byte("hello"), 0o644); err != nil {
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: err.Error()}
		}
		// Commit inside the worktree.
		for _, args := range [][]string{
			{"add", "new.txt"},
			{"commit", "-m", "add new.txt"},
		} {
			cmd := exec.Command("git", args...)
			cmd.Dir = worktreeDir
			if out, err := cmd.CombinedOutput(); err != nil {
				return ToolResult{CallID: call.ID, Name: call.Name, Success: false,
					Error: "git " + strings.Join(args, " ") + ": " + err.Error() + "\n" + string(out)}
			}
		}
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "written"}
	}

	iso, err := NewWorktreeIsolator(repoRoot, handler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}

	result := iso.Execute(context.Background(), ToolCall{ID: "call-1", Name: "write_file"})
	if !result.Success {
		t.Fatalf("Execute failed: %s", result.Error)
	}

	// HEAD must have advanced (merge happened).
	headAfter := currentHead(t, repoRoot)
	if headAfter == headBefore {
		t.Error("expected HEAD to advance after merge, but it did not")
	}
	// The new file must be visible in the main worktree after merge.
	if !fileExistsInRepo(t, repoRoot, "new.txt") {
		t.Error("expected new.txt to exist in main repo after merge")
	}
	// The worktree temp directory must be cleaned up.
	worktreePath := filepath.Join(repoRoot, ".git", "worktrees-tmp", worktreeBranchName("call-1"))
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir %s to be removed, got err=%v", worktreePath, err)
	}
}

func TestWorktreeIsolator_AutoCommitsUncommittedWrites(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()
	handler := func(_ context.Context, worktreeDir string, call ToolCall) ToolResult {
		if err := os.WriteFile(filepath.Join(worktreeDir, "auto.txt"), []byte("n"), 0o644); err != nil {
			return ToolResult{CallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	}
	iso, err := NewWorktreeIsolator(repoRoot, handler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}
	result := iso.Execute(context.Background(), ToolCall{ID: "auto-1", Name: "save_file"})
	if !result.Success {
		t.Fatalf("Execute failed: %s", result.Error)
	}
	if !fileExistsInRepo(t, repoRoot, "auto.txt") {
		t.Fatal("expected auto.txt after isolator auto-commit + merge")
	}
}

// TestWorktreeIsolator_RemovesWorktreeOnFailure verifies that when the handler
// returns a failed result, the worktree is discarded and the base branch is
// unchanged (no merge).
func TestWorktreeIsolator_RemovesWorktreeOnFailure(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()

	headBefore := currentHead(t, repoRoot)

	// Handler always fails.
	handler := func(ctx context.Context, worktreeDir string, call ToolCall) ToolResult {
		return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: "simulated failure"}
	}

	iso, err := NewWorktreeIsolator(repoRoot, handler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}

	result := iso.Execute(context.Background(), ToolCall{ID: "fail-1", Name: "write_file"})
	if result.Success {
		t.Fatal("expected Execute to return failure, got success")
	}
	if !strings.Contains(result.Error, "simulated failure") {
		t.Errorf("expected error to contain 'simulated failure', got %q", result.Error)
	}

	// HEAD must NOT have advanced (no merge on failure).
	headAfter := currentHead(t, repoRoot)
	if headAfter != headBefore {
		t.Errorf("expected HEAD unchanged after failure, got before=%s after=%s",
			headBefore, headAfter)
	}
	// The worktree temp directory must be cleaned up.
	worktreePath := filepath.Join(repoRoot, ".git", "worktrees-tmp", worktreeBranchName("fail-1"))
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir %s to be removed on failure, got err=%v",
			worktreePath, err)
	}
}

// TestWorktreeIsolator_NilIsolatorReturnsError verifies nil safety.
func TestWorktreeIsolator_NilIsolatorReturnsError(t *testing.T) {
	var iso *WorktreeIsolator
	result := iso.Execute(context.Background(), ToolCall{ID: "x", Name: "y"})
	if result.Success {
		t.Error("expected failure for nil isolator")
	}
	if !strings.Contains(result.Error, "not initialized") {
		t.Errorf("expected 'not initialized' error, got %q", result.Error)
	}
}

// TestWorktreeIsolator_EmptyRepoRootReturnsError verifies constructor validation.
func TestWorktreeIsolator_EmptyRepoRootReturnsError(t *testing.T) {
	_, err := NewWorktreeIsolator("", nil, loggateway.NewNoop())
	if err == nil {
		t.Error("expected error for empty repoRoot, got nil")
	}
}

// TestWorktreeIsolator_NilHandlerSucceeds verifies that a nil handler still
// produces a success result (worktree lifecycle only, no file changes).
func TestWorktreeIsolator_NilHandlerSucceeds(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()

	headBefore := currentHead(t, repoRoot)

	iso, err := NewWorktreeIsolator(repoRoot, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}

	result := iso.Execute(context.Background(), ToolCall{ID: "noop-1", Name: "noop"})
	if !result.Success {
		t.Fatalf("expected success for nil handler, got: %s", result.Error)
	}

	// HEAD must NOT have advanced (no commits to merge).
	headAfter := currentHead(t, repoRoot)
	if headAfter != headBefore {
		t.Errorf("expected HEAD unchanged for nil handler, before=%s after=%s",
			headBefore, headAfter)
	}
}

// TestWorktreeBranchNameSanitization verifies that branch names are sanitized.
func TestWorktreeBranchNameSanitization(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"abc", "parallel-exec/abc"},
		{"a/b", "parallel-exec/a-b"},
		{"a b", "parallel-exec/a-b"},
		{"a.b-c_d", "parallel-exec/a.b-c_d"},
		{"café", "parallel-exec/caf-"},
		{"", "parallel-exec/"},
	}
	for _, c := range cases {
		got := worktreeBranchName(c.input)
		if got != c.want {
			t.Errorf("worktreeBranchName(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestWorktreeIsolator_ContextCancelAbortsCreate verifies that a cancelled
// context surfaces as a failed result (create worktree fails or ctx error).
func TestWorktreeIsolator_ContextCancelAbortsCreate(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()

	iso, err := NewWorktreeIsolator(repoRoot, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result := iso.Execute(ctx, ToolCall{ID: "cancel-1", Name: "x"})
	if result.Success {
		t.Error("expected failure for cancelled context")
	}
}

// TestWorktreeIsolator_CleanupRunsAfterContextCancel verifies that when the
// caller's context is cancelled while the handler runs, the deferred cleanup
// still removes the worktree dir and branch — cleanup uses a detached
// (context.WithoutCancel) context, so cancellation must not leak artifacts.
func TestWorktreeIsolator_CleanupRunsAfterContextCancel(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	handler := func(ctx context.Context, worktreeDir string, call ToolCall) ToolResult {
		cancel() // simulate cancellation while the handler runs
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "ok"}
	}

	iso, err := NewWorktreeIsolator(repoRoot, handler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}

	result := iso.Execute(ctx, ToolCall{ID: "cancel-cleanup", Name: "write_file"})
	// The merge runs with the cancelled ctx and fails; the call reports failure.
	if result.Success {
		t.Fatal("expected failure after context cancellation")
	}
	// But cleanup must still have removed the worktree dir.
	worktreePath := filepath.Join(repoRoot, ".git", "worktrees-tmp", worktreeBranchName("cancel-cleanup"))
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir removed after ctx cancel, got err=%v", err)
	}
	// And the branch must be deleted as well.
	cmd := exec.Command("git", "branch", "--list", worktreeBranchName("cancel-cleanup"))
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("expected branch deleted after ctx cancel, got %q", out)
	}
}

// TestBatchExecuteSpiritTools_ParallelWorktreeFileOps is the Phase C end-to-end
// acceptance test: two worktree-tagged file-write calls run concurrently via
// BatchExecuteSpiritTools; each commits a distinct file inside its own
// worktree, and both changes must land in the main repo (ff merge for the
// first, --no-ff merge commit for the second) without corrupting each other.
func TestBatchExecuteSpiritTools_ParallelWorktreeFileOps(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()

	// WorktreeHandler: each call writes the file named by its Arguments
	// (JSON string) into the worktree and commits it there.
	wtHandler := func(ctx context.Context, worktreeDir string, call ToolCall) ToolResult {
		var name string
		if err := json.Unmarshal(call.Arguments, &name); err != nil || name == "" {
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: "bad arguments"}
		}
		if err := os.WriteFile(filepath.Join(worktreeDir, name), []byte("data-"+call.ID), 0o644); err != nil {
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: err.Error()}
		}
		for _, args := range [][]string{
			{"add", name},
			{"commit", "-m", "add " + name},
		} {
			cmd := exec.Command("git", args...)
			cmd.Dir = worktreeDir
			if out, err := cmd.CombinedOutput(); err != nil {
				return ToolResult{CallID: call.ID, Name: call.Name, Success: false,
					Error: "git " + strings.Join(args, " ") + ": " + err.Error() + "\n" + string(out)}
			}
		}
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "written " + name}
	}

	iso, err := NewWorktreeIsolator(repoRoot, wtHandler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}
	parExec := NewParallelToolExecutor(nil, loggateway.NewNoop(),
		WithMaxConcurrency(2), WithWorktreeIsolator(iso))

	directHandler := func(_ context.Context, call ToolCall) ToolResult {
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "direct"}
	}
	calls := []ToolCall{
		{ID: "f1", Name: "save_file", Arguments: json.RawMessage(`"f1.txt"`),
			IsolationStrategy: IsolationStrategyForTool("save_file")},
		{ID: "f2", Name: "save_file", Arguments: json.RawMessage(`"f2.txt"`),
			IsolationStrategy: IsolationStrategyForTool("save_file")},
	}

	headBefore := currentHead(t, repoRoot)
	results := BatchExecuteSpiritTools(context.Background(), parExec, directHandler, calls, loggateway.NewNoop())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("call %s failed: %s", r.CallID, r.Error)
		}
	}
	// Both parallel edits must be merged into the main repo.
	for _, name := range []string{"f1.txt", "f2.txt"} {
		if !fileExistsInRepo(t, repoRoot, name) {
			t.Errorf("expected %s to exist in main repo after parallel merge", name)
		}
	}
	if headAfter := currentHead(t, repoRoot); headAfter == headBefore {
		t.Error("expected HEAD to advance after parallel merges")
	}
}
