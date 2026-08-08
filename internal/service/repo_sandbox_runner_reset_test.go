package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ResetWorktree（R1 重试回路一致性）：每次 Patcher 尝试前把 worktree 恢复到
// base-ref 状态——reset --hard 清已跟踪改动，clean -fd 清未跟踪文件，保证
// 重试验证的是 run.Diff 而非前次尝试的叠加态。
func TestRepoSandboxRunner_ResetWorktree(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-reset", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	// 污染 worktree：apply 一个 diff（修改已跟踪文件）+ 落一个未跟踪文件。
	if err := r.ApplyDiff(context.Background(), path, fixtureMainPatch); err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	writeFile(t, path, "scratch.txt", "untracked scratch")
	if got := readFile(t, path, "main.go"); !strings.Contains(got, "patched") {
		t.Fatalf("precondition: main.go not patched: %q", got)
	}

	if err := r.ResetWorktree(context.Background(), path); err != nil {
		t.Fatalf("ResetWorktree: %v", err)
	}
	// reset --hard：已跟踪文件回到 base-ref 内容。
	if got := readFile(t, path, "main.go"); strings.Contains(got, "patched") {
		t.Errorf("main.go still patched after reset: %q", got)
	}
	// clean -fd：未跟踪文件被移除。
	if _, err := os.Stat(filepath.Join(path, "scratch.txt")); !os.IsNotExist(err) {
		t.Errorf("untracked scratch.txt should be cleaned, stat err = %v", err)
	}
	// reset 后同一 diff 可再次干净 apply（重试回路核心断言）。
	if err := r.ApplyDiff(context.Background(), path, fixtureMainPatch); err != nil {
		t.Fatalf("ApplyDiff after reset: %v", err)
	}
}
