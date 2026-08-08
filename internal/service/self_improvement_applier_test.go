package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func gitMust(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

func newApplierFixture(t *testing.T, repo string) *SIRepoApplier {
	t.Helper()
	a, err := NewSIRepoApplier(newTestRunner(t, repo), loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewSIRepoApplier: %v", err)
	}
	return a
}

// initHotReloadFixture extends the go fixture with config/prompt files.
func initHotReloadFixture(t *testing.T) string {
	t.Helper()
	repo := initFixtureGoRepo(t)
	writeFile(t, repo, "configs/app.yaml", "key: v1\n")
	writeFile(t, repo, "prompts/old.txt", "old prompt\n")
	gitMust(t, repo, "add", ".")
	gitMust(t, repo, "commit", "-m", "add config and prompts")
	return repo
}

const hotReloadDiff = `diff --git a/configs/app.yaml b/configs/app.yaml
index 3b18e51..6f7e0b2 100644
--- a/configs/app.yaml
+++ b/configs/app.yaml
@@ -1 +1 @@
-key: v1
+key: v2
diff --git a/prompts/new.txt b/prompts/new.txt
new file mode 100644
index 0000000..3e75765
--- /dev/null
+++ b/prompts/new.txt
@@ -0,0 +1 @@
+new prompt
diff --git a/prompts/old.txt b/prompts/old.txt
deleted file mode 100644
index 9daeafb..0000000
--- a/prompts/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-old prompt
`

// ── ApplyHotReload ───────────────────────────────────────────────────────────

func TestSIRepoApplier_ApplyHotReload_ModifyCreateDeleteThenRollback(t *testing.T) {
	repo := initHotReloadFixture(t)
	a := newApplierFixture(t, repo)

	run := &biz.SelfImprovementRun{ID: "run-hr1", PatchKind: biz.PatchKindConfig, Diff: hotReloadDiff}
	ref, err := a.ApplyHotReload(context.Background(), run)
	if err != nil {
		if ae, ok := apierror.From(err); ok {
			t.Fatalf("ApplyHotReload: %v | meta=%v", err, ae.Meta)
		}
		t.Fatalf("ApplyHotReload: %v", err)
	}
	if ref == "" {
		t.Fatal("rollbackRef is empty")
	}

	// Working tree carries the patch (no commit on the hot-reload channel).
	if got := readFile(t, repo, "configs/app.yaml"); got != "key: v2\n" {
		t.Errorf("app.yaml = %q, want patched", got)
	}
	if got := readFile(t, repo, "prompts/new.txt"); got != "new prompt\n" {
		t.Errorf("new.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "prompts", "old.txt")); !os.IsNotExist(err) {
		t.Errorf("old.txt should be deleted, stat err = %v", err)
	}

	// Snapshot exists for rollback.
	if _, err := os.Stat(filepath.Join(repo, defaultWorktreeRoot, "snapshots")); err != nil {
		t.Errorf("snapshots root missing: %v", err)
	}

	// Rollback restores the pre-apply state and consumes the snapshot.
	run.RollbackPointer = ref
	if err := a.Rollback(context.Background(), run, "manual"); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if got := readFile(t, repo, "configs/app.yaml"); got != "key: v1\n" {
		t.Errorf("after rollback app.yaml = %q, want original", got)
	}
	if got := readFile(t, repo, "prompts/old.txt"); got != "old prompt\n" {
		t.Errorf("after rollback old.txt = %q, want restored", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "prompts", "new.txt")); !os.IsNotExist(err) {
		t.Errorf("created file should be removed on rollback, stat err = %v", err)
	}

	// R3 契约变更（2026-08-08）：double rollback 幂等返回 nil（目标状态已达
	// 成），不再依赖"快照已消费"报错。幂等跳过只在本进程确认 restore 成功
	// 后触发（reverted 集合），不会掩盖真实失败；进程重启后集合丢失，第二
	// 次 restore 仍因快照缺失而报错（残余窗口见 applier 注释）。
	if err := a.Rollback(context.Background(), run, "again"); err != nil {
		t.Errorf("second Rollback should be idempotent nil, got: %v", err)
	}
	// 幂等跳过无副作用：文件系统仍保持已回滚状态。
	if got := readFile(t, repo, "configs/app.yaml"); got != "key: v1\n" {
		t.Errorf("after idempotent skip app.yaml = %q, want still reverted", got)
	}
}

func TestSIRepoApplier_ApplyHotReload_RejectsPathTraversal(t *testing.T) {
	repo := initHotReloadFixture(t)
	a := newApplierFixture(t, repo)

	diff := "diff --git a/../evil.txt b/../evil.txt\n" +
		"new file mode 100644\n" +
		"index 0000000..f719efd\n" +
		"--- /dev/null\n" +
		"+++ b/../evil.txt\n" +
		"@@ -0,0 +1 @@\n" +
		"+pwned\n"
	run := &biz.SelfImprovementRun{ID: "run-hr2", PatchKind: biz.PatchKindConfig, Diff: diff}
	if _, err := a.ApplyHotReload(context.Background(), run); err == nil {
		t.Fatal("path traversal diff should fail")
	}
	if _, err := os.Stat(filepath.Join(repo, "..", "evil.txt")); !os.IsNotExist(err) {
		t.Error("traversal file was created outside the repo")
	}
	if _, err := os.Stat(filepath.Join(repo, defaultWorktreeRoot, "snapshots")); !os.IsNotExist(err) {
		t.Error("snapshot dir should not be left behind on rejected diff")
	}
}

func TestSIRepoApplier_ApplyHotReload_ContextMismatchLeavesNoTrace(t *testing.T) {
	repo := initHotReloadFixture(t)
	a := newApplierFixture(t, repo)

	diff := "diff --git a/configs/app.yaml b/configs/app.yaml\n" +
		"index 3b18e51..6f7e0b2 100644\n" +
		"--- a/configs/app.yaml\n" +
		"+++ b/configs/app.yaml\n" +
		"@@ -1 +1 @@\n" +
		"-key: WRONG\n" +
		"+key: v2\n"
	run := &biz.SelfImprovementRun{ID: "run-hr3", PatchKind: biz.PatchKindConfig, Diff: diff}
	if _, err := a.ApplyHotReload(context.Background(), run); err == nil {
		t.Fatal("context-mismatched diff should fail")
	}
	if got := readFile(t, repo, "configs/app.yaml"); got != "key: v1\n" {
		t.Errorf("working tree modified despite failed apply: %q", got)
	}
	entries, _ := os.ReadDir(filepath.Join(repo, defaultWorktreeRoot, "snapshots"))
	if len(entries) != 0 {
		t.Errorf("stale snapshot left behind: %v", entries)
	}
}

func TestSIRepoApplier_ApplyRejectsEmptyDiff(t *testing.T) {
	repo := initHotReloadFixture(t)
	a := newApplierFixture(t, repo)
	run := &biz.SelfImprovementRun{ID: "run-hr4", PatchKind: biz.PatchKindConfig}
	if _, err := a.ApplyHotReload(context.Background(), run); err == nil {
		t.Error("empty diff should fail (hot-reload)")
	}
	run.PatchKind = biz.PatchKindCode
	if _, err := a.ApplyCodeMerge(context.Background(), run); err == nil {
		t.Error("empty diff should fail (code merge)")
	}
}

// ── ApplyCodeMerge ───────────────────────────────────────────────────────────

const codeMergeDiff = `diff --git a/main.go b/main.go
index 5d11ff6..e9ce9d1 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package main

-func main() { println("hi") }
+func main() { println("applied") }
diff --git a/docs/note.md b/docs/note.md
new file mode 100644
index 0000000..2f259b7
--- /dev/null
+++ b/docs/note.md
@@ -0,0 +1 @@
+# note
`

func TestSIRepoApplier_ApplyCodeMerge_FastForward(t *testing.T) {
	repo := initFixtureGoRepo(t)
	a := newApplierFixture(t, repo)
	headBefore := gitOut(t, repo, "rev-parse", "HEAD")

	run := &biz.SelfImprovementRun{ID: "run-cm1", PatchKind: biz.PatchKindCode, Diff: codeMergeDiff}
	sha, err := a.ApplyCodeMerge(context.Background(), run)
	if err != nil {
		t.Fatalf("ApplyCodeMerge: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("commitSHA = %q, want 40-hex", sha)
	}

	// Main branch fast-forwarded to the patch commit.
	if got := gitOut(t, repo, "rev-parse", "HEAD"); got != sha {
		t.Errorf("HEAD = %q, want %q", got, sha)
	}
	if gitOut(t, repo, "rev-parse", "HEAD~1") != headBefore {
		t.Error("merge was not a fast-forward (history rewritten)")
	}

	// Commit message carries the self-improvement markers (design D7).
	msg := gitOut(t, repo, "log", "-1", "--format=%B")
	if !strings.Contains(msg, "self-improvement: true") {
		t.Errorf("commit message missing self-improvement trailer: %q", msg)
	}
	if !strings.Contains(msg, "run-id: run-cm1") {
		t.Errorf("commit message missing run-id trailer: %q", msg)
	}

	// Patch content landed; worktree + branch are gone.
	if got := readFile(t, repo, "main.go"); !strings.Contains(got, "applied") {
		t.Errorf("main.go not patched: %q", got)
	}
	if got := readFile(t, repo, "docs/note.md"); got != "# note\n" {
		t.Errorf("note.md = %q", got)
	}
	if gitBranchExists(t, repo, "self-improve/run-cm1") {
		t.Error("patch branch should be deleted after merge")
	}
	entries, _ := os.ReadDir(filepath.Join(repo, defaultWorktreeRoot))
	for _, e := range entries {
		if e.Name() != "snapshots" {
			t.Errorf("stale worktree entry left behind: %s", e.Name())
		}
	}
}

func TestSIRepoApplier_ApplyCodeMerge_DriftConflictFailsCleanly(t *testing.T) {
	repo := initFixtureGoRepo(t)
	a := newApplierFixture(t, repo)

	// Drift: main branch moved to content the patch context no longer matches.
	writeFile(t, repo, "main.go", "package main\n\nfunc main() { println(\"drifted\") }\n")
	gitMust(t, repo, "add", ".")
	gitMust(t, repo, "commit", "-m", "drift")
	headDrifted := gitOut(t, repo, "rev-parse", "HEAD")

	run := &biz.SelfImprovementRun{ID: "run-cm2", PatchKind: biz.PatchKindCode, Diff: codeMergeDiff}
	_, err := a.ApplyCodeMerge(context.Background(), run)
	if err == nil {
		t.Fatal("drifted patch should fail")
	}
	if !errors.Is(err, biz.ErrSIMergeConflict) {
		t.Errorf("error = %v, want errors.Is ErrSIMergeConflict", err)
	}

	// Nothing landed: HEAD untouched, no branch, no worktree.
	if got := gitOut(t, repo, "rev-parse", "HEAD"); got != headDrifted {
		t.Errorf("HEAD moved despite failed apply: %q", got)
	}
	if got := readFile(t, repo, "main.go"); !strings.Contains(got, "drifted") {
		t.Errorf("main repo working tree clobbered: %q", got)
	}
	if gitBranchExists(t, repo, "self-improve/run-cm2") {
		t.Error("stale branch left behind")
	}
}

func TestSIRepoApplier_Rollback_CodeRevert(t *testing.T) {
	repo := initFixtureGoRepo(t)
	a := newApplierFixture(t, repo)

	run := &biz.SelfImprovementRun{ID: "run-cm3", PatchKind: biz.PatchKindCode, Diff: codeMergeDiff}
	sha, err := a.ApplyCodeMerge(context.Background(), run)
	if err != nil {
		t.Fatalf("ApplyCodeMerge: %v", err)
	}
	run.AppliedCommit = sha

	if err := a.Rollback(context.Background(), run, "watchdog: error rate +80%"); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	head := gitOut(t, repo, "rev-parse", "HEAD")
	if head == sha {
		t.Error("revert should create a new commit on top")
	}
	if subj := gitOut(t, repo, "log", "-1", "--format=%s"); !strings.Contains(subj, "Revert") {
		t.Errorf("revert commit subject = %q", subj)
	}
	if got := readFile(t, repo, "main.go"); !strings.Contains(got, `"hi"`) {
		t.Errorf("main.go not reverted: %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "note.md")); !os.IsNotExist(err) {
		t.Error("added file should be removed by revert")
	}
}

func TestSIRepoApplier_Rollback_IdempotentDoubleRevert(t *testing.T) {
	repo := initFixtureGoRepo(t)
	a := newApplierFixture(t, repo)

	run := &biz.SelfImprovementRun{ID: "run-cm5", PatchKind: biz.PatchKindCode, Diff: codeMergeDiff}
	sha, err := a.ApplyCodeMerge(context.Background(), run)
	if err != nil {
		t.Fatalf("ApplyCodeMerge: %v", err)
	}
	run.AppliedCommit = sha

	// 第一次 rollback：正常 revert。
	if err := a.Rollback(context.Background(), run, "watchdog: regression"); err != nil {
		t.Fatalf("first Rollback: %v", err)
	}
	headAfterFirst := gitOut(t, repo, "rev-parse", "HEAD")

	// R3：admin 与 watchdog 基于同一 observing 快照并发进入 Rollback 时，
	// 第二次调用必须幂等——否则 git revert 一个已 revert 的 commit 会生成
	// revert-of-revert，已回滚的补丁静默复活（DB 却显示 rolled_back）。
	if err := a.Rollback(context.Background(), run, "admin concurrent"); err != nil {
		t.Fatalf("second Rollback should be idempotent nil: %v", err)
	}
	if got := gitOut(t, repo, "rev-parse", "HEAD"); got != headAfterFirst {
		t.Errorf("double rollback moved HEAD (%s → %s): revert-of-revert resurrected the patch", headAfterFirst, got)
	}
	if got := readFile(t, repo, "main.go"); !strings.Contains(got, `"hi"`) {
		t.Errorf("main.go should stay reverted: %q", got)
	}
}

func TestSIRepoApplier_Rollback_NothingToRollBack(t *testing.T) {
	repo := initFixtureGoRepo(t)
	a := newApplierFixture(t, repo)
	run := &biz.SelfImprovementRun{ID: "run-cm4"}
	if err := a.Rollback(context.Background(), run, "no-op"); err == nil {
		t.Error("rollback with neither commit nor snapshot should fail")
	}
}
