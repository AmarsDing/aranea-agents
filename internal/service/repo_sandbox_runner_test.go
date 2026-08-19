package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ── fixture helpers ──────────────────────────────────────────────────────────

// initFixtureGoRepo creates a temporary git repository containing a minimal
// buildable go module. Skips when git or go is unavailable.
func initFixtureGoRepo(t *testing.T) string {
	t.Helper()
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available on PATH: %v", tool, err)
		}
	}
	root := t.TempDir()
	run := func(dir, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = fixtureCmdEnv(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
	}

	run(root, "git", "init")
	run(root, "git", "config", "user.email", "test@example.com")
	run(root, "git", "config", "user.name", "Test")
	run(root, "git", "config", "commit.gpgsign", "false")
	run(root, "git", "config", "core.autocrlf", "false")

	writeFile(t, root, "go.mod", "module example.com/fixture\n\ngo 1.21\n")
	writeFile(t, root, "main.go", "package main\n\nfunc main() { println(\"hi\") }\n")
	writeFile(t, root, "pkg/foo/foo.go", "package foo\n\n// Answer returns 42.\nfunc Answer() int { return 42 }\n")
	run(root, "git", "add", ".")
	run(root, "git", "commit", "-m", "initial")
	return root
}

// fixtureCmdEnv isolates GOCACHE per test to avoid cache locks between
// parallel sessions (project memory: phantom build errors).
func fixtureCmdEnv(t *testing.T) []string {
	t.Helper()
	env := os.Environ()
	env = append(env, "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))
	return env
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", rel, err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", rel, err)
	}
	return string(b)
}

func gitBranchExists(t *testing.T, repoRoot, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", branch)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	return strings.TrimSpace(string(out)) != ""
}

func newTestRunner(t *testing.T, repoRoot string) *RepoSandboxRunner {
	t.Helper()
	r, err := NewRepoSandboxRunner(repoRoot, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewRepoSandboxRunner: %v", err)
	}
	return r
}

// ── PrepareWorktree / cleanup ────────────────────────────────────────────────

func TestRepoSandboxRunner_PrepareAndCleanup(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)

	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-abc123", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "main.go")); err != nil {
		t.Errorf("worktree missing main.go: %v", err)
	}
	if !gitBranchExists(t, repo, "self-improve/run-abc123") {
		t.Error("expected branch self-improve/run-abc123 to exist")
	}
	// The worktree must sit under the configured worktree root.
	if !strings.Contains(path, ".aranea-self-improve") {
		t.Errorf("worktree path %q not under .aranea-self-improve", path)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after cleanup: %v", err)
	}
	if gitBranchExists(t, repo, "self-improve/run-abc123") {
		t.Error("branch still exists after cleanup")
	}
}

func TestRepoSandboxRunner_CleanupIdempotentAndSurvivesCancel(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)

	ctx, cancel := context.WithCancel(context.Background())
	path, cleanup, err := r.PrepareWorktree(ctx, "run-idem", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	cancel() // caller context dead — cleanup must still work
	cleanup()
	cleanup() // second call must be a no-op, not a failure/panic
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists: %v", err)
	}
}

func TestRepoSandboxRunner_PrepareDuplicateRunIDFails(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)

	_, cleanup, err := r.PrepareWorktree(context.Background(), "run-dup", "")
	if err != nil {
		t.Fatalf("first PrepareWorktree: %v", err)
	}
	defer cleanup()
	if _, _, err := r.PrepareWorktree(context.Background(), "run-dup", ""); err == nil {
		t.Error("duplicate runID should fail")
	}
}

// 回归（2026-08-19 运行时事故）：git 注册 worktree 后、checkout 完成前被杀
//（exit 255），PrepareWorktree 返回错误但半截 worktree+分支永久泄漏。新增
// purgeRegisteredWorktree 负责摘除该注册态；这里直接对原语做确定性验证。
func TestRepoSandboxRunner_PurgeRegisteredWorktree(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	ctx := context.Background()

	// 手工构造"已注册的半截 worktree"（等价于 git 被杀瞬间的状态）。
	wtPath := filepath.Join(repo, ".aranea-self-improve", "run-half")
	cmd := exec.Command("git", "-c", "core.longpaths=true", "worktree", "add", "-b", "self-improve/run-half", wtPath, "HEAD")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setup worktree: %v\n%s", err, out)
	}
	if !r.worktreeRegistered(ctx, wtPath) {
		t.Fatal("setup: worktree should be registered")
	}

	r.purgeRegisteredWorktree(ctx, "self-improve/run-half", wtPath)

	if r.worktreeRegistered(ctx, wtPath) {
		t.Error("worktree still registered after purge")
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after purge: %v", err)
	}
	cmd = exec.Command("git", "rev-parse", "--verify", "self-improve/run-half")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("branch still exists after purge: %s", out)
	}
}

// 边界守卫：add 报错且 wtPath 在调用前已被他人注册（活跃占用），不得清理。
func TestRepoSandboxRunner_PrepareKeepsForeignRegisteredWorktree(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	ctx := context.Background()

	// 外来占用：同路径、不同分支的已注册 worktree。
	wtPath := filepath.Join(repo, ".aranea-self-improve", "run-foreign")
	cmd := exec.Command("git", "-c", "core.longpaths=true", "worktree", "add", "-b", "other/foreign", wtPath, "HEAD")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setup foreign worktree: %v\n%s", err, out)
	}
	defer func() {
		c := exec.Command("git", "-c", "core.longpaths=true", "worktree", "remove", "--force", wtPath)
		c.Dir = repo
		_ = c.Run()
	}()

	if _, _, err := r.PrepareWorktree(ctx, "run-foreign", ""); err == nil {
		t.Fatal("PrepareWorktree on foreign-registered path must fail")
	}
	if !r.worktreeRegistered(ctx, wtPath) {
		t.Error("foreign worktree must not be purged by failed PrepareWorktree")
	}
}

// 回归（2026-08-08 运行时事故）：prepare 在分支创建后/checkout 中途失败（或进程
// 被杀），留下同名分支 + 残留目录的崩溃孤儿；stale 恢复重驱动时 `worktree add -b`
// 撞 "already exists" 永久 exit 128。PrepareWorktree 必须清理孤儿后自愈重试。
// 注意与 TestRepoSandboxRunner_PrepareDuplicateRunIDFails 的边界：已注册的同名
// worktree（活跃重复 runID）仍必须报错，仅清理未注册的孤儿。
func TestRepoSandboxRunner_PrepareHealsCrashOrphan(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)

	// 构造孤儿：分支已建、worktree 未注册、目录有半截残留文件。
	cmd := exec.Command("git", "branch", "self-improve/run-orphan")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}
	writeFile(t, repo, ".aranea-self-improve/run-orphan/partial.txt", "half-written checkout")

	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-orphan", "")
	if err != nil {
		t.Fatalf("PrepareWorktree must self-heal crash orphan: %v", err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(path, "main.go")); err != nil {
		t.Errorf("healed worktree missing main.go: %v", err)
	}
}

// 回归（2026-08-08 运行时事故）：Windows MAX_PATH(260) —— 沙盒路径前缀
// （repoRoot + .aranea-self-improve + uuid ≈ 76 字符）叠加仓库长路径文件
// （如 bench 输出 229 字符）→ checkout "Filename too long" exit 128，分支成孤儿。
// runGit 必须携带 -c core.longpaths=true（非 Windows 平台无害）。
func TestRepoSandboxRunner_PrepareLongPaths(t *testing.T) {
	repo := initFixtureGoRepo(t)
	// 提交一个深嵌套长文件名文件（总长 > 260 触发 Windows MAX_PATH）。
	longRel := "pkg/deep/" + strings.Repeat("sub/", 20) + strings.Repeat("d", 120) + ".txt"
	writeFile(t, repo, longRel, "deep")
	for _, args := range [][]string{
		{"-c", "core.longpaths=true", "add", "."},
		{"-c", "core.longpaths=true", "commit", "-m", "long path file"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = fixtureCmdEnv(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-longpath", "")
	if err != nil {
		t.Fatalf("PrepareWorktree with long repo paths: %v", err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(path, filepath.FromSlash(longRel))); err != nil {
		t.Errorf("long-path file missing in worktree: %v", err)
	}
}

func TestRepoSandboxRunner_RunIDSanitized(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)

	path, cleanup, err := r.PrepareWorktree(context.Background(), "run/evil ../../x", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()
	absRoot, _ := filepath.Abs(repo)
	absPath, _ := filepath.Abs(path)
	if !strings.HasPrefix(absPath, absRoot) {
		t.Errorf("worktree escaped repo root: %q", path)
	}
	if !gitBranchExists(t, repo, "self-improve/run-evil-x") {
		t.Errorf("expected sanitized branch, branches: %v", path)
	}
}

// ── ApplyDiff ────────────────────────────────────────────────────────────────

const fixtureMainPatch = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package main

-func main() { println("hi") }
+func main() { println("patched") }
`

func TestRepoSandboxRunner_ApplyDiff(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-apply", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	if err := r.ApplyDiff(context.Background(), path, fixtureMainPatch); err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	if got := readFile(t, path, "main.go"); !strings.Contains(got, "patched") {
		t.Errorf("main.go not patched: %q", got)
	}
	// Main repo must stay untouched.
	if got := readFile(t, repo, "main.go"); strings.Contains(got, "patched") {
		t.Error("main repo was modified by sandbox ApplyDiff")
	}
}

func TestRepoSandboxRunner_ApplyDiffRejectsGarbage(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-garbage", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	if err := r.ApplyDiff(context.Background(), path, "this is not a diff"); err == nil {
		t.Error("garbage diff should fail")
	}
}

func TestRepoSandboxRunner_RejectsPathOutsideSandbox(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)

	if err := r.ApplyDiff(context.Background(), repo, fixtureMainPatch); err == nil {
		t.Error("ApplyDiff against repo root (outside worktree root) must fail")
	}
	if _, err := r.RunGate(context.Background(), repo, biz.SandboxGateBuild, nil); err == nil {
		t.Error("RunGate against repo root (outside worktree root) must fail")
	}
}

// ── RunGate G1/G2/G3 ─────────────────────────────────────────────────────────

func TestRepoSandboxRunner_G1BuildPassAndFail(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-g1", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	res, err := r.RunGate(context.Background(), path, biz.SandboxGateBuild, nil)
	if err != nil {
		t.Fatalf("RunGate G1: %v", err)
	}
	if !res.Passed {
		t.Errorf("G1 should pass on clean fixture, output: %s", res.Output)
	}
	if res.DurationMS <= 0 {
		t.Error("G1 result missing duration")
	}

	// Break compilation.
	broken := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package main

-func main() { println("hi") }
+func main() { undefinedSymbol() }
`
	if err := r.ApplyDiff(context.Background(), path, broken); err != nil {
		t.Fatalf("ApplyDiff broken: %v", err)
	}
	res, err = r.RunGate(context.Background(), path, biz.SandboxGateBuild, nil)
	if err != nil {
		t.Fatalf("RunGate G1 broken: %v", err)
	}
	if res.Passed {
		t.Error("G1 should fail on broken patch")
	}
	if !strings.Contains(res.Output, "undefined") {
		t.Errorf("G1 output should carry compiler error, got: %s", res.Output)
	}
}

func TestRepoSandboxRunner_G2TestScopedPkgs(t *testing.T) {
	repo := initFixtureGoRepo(t)
	writeFile(t, repo, "pkg/foo/foo_test.go",
		"package foo\n\nimport \"testing\"\n\nfunc TestAnswer(t *testing.T) {\n\tif Answer() != 42 {\n\t\tt.Fatal(\"wrong\")\n\t}\n}\n")
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "add test")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-g2", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	res, err := r.RunGate(context.Background(), path, biz.SandboxGateTest, []string{"./pkg/foo/..."})
	if err != nil {
		t.Fatalf("RunGate G2: %v", err)
	}
	if !res.Passed {
		t.Errorf("G2 should pass, output: %s", res.Output)
	}

	// Make the test fail.
	breakTest := `diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go
--- a/pkg/foo/foo.go
+++ b/pkg/foo/foo.go
@@ -1,4 +1,4 @@
 package foo

 // Answer returns 42.
-func Answer() int { return 42 }
+func Answer() int { return 41 }
`
	if err := r.ApplyDiff(context.Background(), path, breakTest); err != nil {
		t.Fatalf("ApplyDiff breakTest: %v", err)
	}
	res, err = r.RunGate(context.Background(), path, biz.SandboxGateTest, []string{"./pkg/foo/..."})
	if err != nil {
		t.Fatalf("RunGate G2 failing: %v", err)
	}
	if res.Passed {
		t.Error("G2 should fail when test fails")
	}
}

func TestRepoSandboxRunner_G3Vet(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-g3", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	res, err := r.RunGate(context.Background(), path, biz.SandboxGateLint, []string{"./..."})
	if err != nil {
		t.Fatalf("RunGate G3: %v", err)
	}
	if !res.Passed {
		t.Errorf("G3 vet should pass on clean fixture, output: %s", res.Output)
	}
}

func TestRepoSandboxRunner_UnsupportedGate(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-g4", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	if _, err := r.RunGate(context.Background(), path, biz.SandboxGateCritic, nil); err == nil {
		t.Error("G4/G5 are not runner gates — want explicit error")
	}
}

func TestRepoSandboxRunner_GateTimeout(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r, err := NewRepoSandboxRunner(repo, loggateway.NewNoop(),
		WithGateTimeout(biz.SandboxGateTest, time.Nanosecond))
	if err != nil {
		t.Fatalf("NewRepoSandboxRunner: %v", err)
	}
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-timeout", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	res, err := r.RunGate(context.Background(), path, biz.SandboxGateTest, []string{"./..."})
	if err != nil {
		t.Fatalf("RunGate timeout: %v", err)
	}
	if res.Passed {
		t.Error("1ns timeout must fail the gate")
	}
}

// ── pure helpers ─────────────────────────────────────────────────────────────

func TestTruncateGateOutput(t *testing.T) {
	big := strings.Repeat("x", maxGateOutputBytes+100)
	got := truncateGateOutput(big)
	if len(got) > maxGateOutputBytes+64 { // tail marker allowance
		t.Errorf("truncateGateOutput len = %d, want <= %d", len(got), maxGateOutputBytes+64)
	}
	if !strings.HasPrefix(got, strings.Repeat("x", 100)) {
		t.Error("truncateGateOutput should keep the head of the output")
	}
	if got == big {
		t.Error("no truncation happened")
	}
}

// interface guard
var _ biz.RepoSandbox = (*RepoSandboxRunner)(nil)
