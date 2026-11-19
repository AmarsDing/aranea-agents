package fleet

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ---------- 测试替身 ----------

type memStore struct {
	mu   sync.Mutex
	docs map[string]map[string][]byte
	seq  map[string][]string
}

func newMemStore() *memStore {
	return &memStore{docs: map[string]map[string][]byte{}, seq: map[string][]string{}}
}

func (m *memStore) Put(_ context.Context, kind, id string, doc []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.docs[kind] == nil {
		m.docs[kind] = map[string][]byte{}
	}
	if _, ok := m.docs[kind][id]; !ok {
		m.seq[kind] = append(m.seq[kind], id)
	}
	m.docs[kind][id] = append([]byte(nil), doc...)
	return nil
}

func (m *memStore) Get(_ context.Context, kind, id string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[kind][id]
	return d, ok, nil
}

func (m *memStore) List(_ context.Context, kind string) ([][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out [][]byte
	for _, id := range m.seq[kind] {
		if d, ok := m.docs[kind][id]; ok {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *memStore) Delete(_ context.Context, kind, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.docs[kind], id)
	return nil
}

type fakeProjects struct{ items []Project }

func (f *fakeProjects) ListProjects(context.Context) ([]Project, error) { return f.items, nil }
func (f *fakeProjects) AddProject(_ context.Context, name, path string) (Project, error) {
	p := Project{ID: "p" + name, Name: name, Path: path}
	f.items = append(f.items, p)
	return p, nil
}

func gitInit(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "fleet@test.local"},
		{"config", "user.name", "fleet test"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := runGit(context.Background(), dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n\nfunc A() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(context.Background(), dir, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(context.Background(), dir, "commit", "-q", "-m", "init"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newTestUsecase(t *testing.T, repo string) *Usecase {
	t.Helper()
	u := NewUsecase(newMemStore(), &fakeProjects{items: []Project{{ID: "p1", Name: "demo", Path: repo}}},
		Options{WorktreeRoot: t.TempDir(), ServerURL: "http://127.0.0.1:1", CLIDir: t.TempDir()}, loggateway.NewNoop())
	t.Cleanup(u.Close)
	return u
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// ---------- 纯函数 ----------

func TestParseUnifiedDiff(t *testing.T) {
	raw := `diff --git a/app.go b/app.go
index 1111111..2222222 100644
--- a/app.go
+++ b/app.go
@@ -1,3 +1,4 @@
 package app

-func A() int { return 1 }
+func A() int { return 2 }
+func B() int { return 3 }
diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+hello
diff --git a/old.txt b/old.txt
deleted file mode 100644
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
diff --git a/a.txt b/b.txt
similarity index 100%
rename from a.txt
rename to b.txt
`
	files := parseUnifiedDiff(raw)
	if len(files) != 4 {
		t.Fatalf("want 4 files, got %d", len(files))
	}
	f := files[0]
	if f.Path != "app.go" || f.Status != "modified" || f.Additions != 2 || f.Deletions != 1 {
		t.Fatalf("modified file parsed wrong: %+v", f)
	}
	lines := f.Hunks[0].Lines
	if lines[2].Kind != "del" || lines[2].OldNo != 3 || lines[3].Kind != "add" || lines[3].NewNo != 3 || lines[4].NewNo != 4 {
		t.Fatalf("line numbers wrong: %+v", lines)
	}
	if files[1].Status != "added" || files[1].Path != "new.txt" {
		t.Fatalf("added file wrong: %+v", files[1])
	}
	if files[2].Status != "deleted" || files[2].Path != "old.txt" {
		t.Fatalf("deleted file wrong: %+v", files[2])
	}
	if files[3].Status != "renamed" || files[3].OldPath != "a.txt" || files[3].Path != "b.txt" {
		t.Fatalf("renamed file wrong: %+v", files[3])
	}
}

func TestScheduleNext(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.Local) // 周五
	cases := []struct {
		spec string
		want time.Time
	}{
		{"daily 02:00", time.Date(2026, 11, 21, 2, 0, 0, 0, time.Local)},
		{"daily 11:30", time.Date(2026, 11, 20, 11, 30, 0, 0, time.Local)},
		{"weekdays 09:00", time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)},
		{"weekly mon 09:00", time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)},
		{"every 6h", base.Add(6 * time.Hour)},
	}
	for _, c := range cases {
		s, err := parseSchedule(c.spec)
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if got := s.next(base); !got.Equal(c.want) {
			t.Fatalf("%s: want %v got %v", c.spec, c.want, got)
		}
	}
	for _, bad := range []string{"", "daily 25:00", "every 1m", "weekly xyz 09:00", "monthly 1"} {
		if _, err := parseSchedule(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

// 切换 worktree / 调整窗格会让终端重绘一屏：这不是 agent 在干活，
// 状态不能从「完成」跳回「工作中」（否则随后还会多发一条「完成了一轮」通知）。
func TestRepaintAfterResizeIsNotWork(t *testing.T) {
	repaint := []byte(strings.Repeat("redrawn line of terminal content\r\n", 40))
	statusAfter := func(resized bool) string {
		s := newSession("t", "tok", sessionSpec{Agent: "codex"}, nil)
		s.status, s.firstOutput, s.prompted = StatusDone, true, true
		s.cols, s.rows, s.statusChangedAt = 120, 32, time.Now()
		if resized {
			s.Resize(100, 30)
		}
		s.appendOutput(repaint)
		s.tick(time.Now())
		return s.status
	}
	if got := statusAfter(true); got != StatusDone {
		t.Fatalf("repaint after resize: want done, got %s", got)
	}
	if got := statusAfter(false); got != StatusWorking {
		t.Fatalf("same output without resize is real work: want working, got %s", got)
	}
	// 尺寸没变就不该再动终端（也不会开启重绘豁免）。
	s := newSession("t", "tok", sessionSpec{Agent: "codex"}, nil)
	s.cols, s.rows = 100, 30
	s.Resize(100, 30)
	if !s.resizedAt.IsZero() {
		t.Fatal("resize to the same size must be a no-op")
	}
}

// 确认提示在屏幕最后几行 → 等你；答过之后提示被后续输出顶上去 → 不再算等你。
func TestNeedsInputOnlyWhilePromptIsLast(t *testing.T) {
	statusAfter := func(screen string) string {
		now := time.Now()
		s := newSession("t", "tok", sessionSpec{Agent: "codex"}, nil)
		s.status, s.firstOutput, s.prompted = StatusWorking, true, true
		s.tail, s.lastOutput, s.statusChangedAt = []byte(screen), now.Add(-5*time.Second), now.Add(-5*time.Second)
		s.tick(now)
		return s.status
	}
	pending := "running tests\r\nDo you want to proceed? (y/n) "
	if got := statusAfter(pending); got != StatusNeedsInput {
		t.Fatalf("pending prompt: want needs_input, got %s", got)
	}
	answered := pending + "y\r\n" + strings.Repeat("  step output line\r\n", 8) + "done.\r\nagent> "
	if got := statusAfter(answered); got != StatusDone {
		t.Fatalf("answered prompt: want done, got %s", got)
	}
	// 真实 Claude Code v2.1.318 的「是否信任此目录」画面（2026-11-18 实测时没被认出来）。
	trust := " Claude Code'll be able to read, edit, and execute files here.\r\n Security guide\r\n ❯ No, exit\r\n   Yes, I trust this folder\r\n Enter to confirm · Esc to cancel\r\n"
	if got := statusAfter(trust); got != StatusNeedsInput {
		t.Fatalf("trust-folder dialog: want needs_input, got %s", got)
	}
}

func TestStripANSIAndDetection(t *testing.T) {
	raw := "\x1b]0;⠙ Working on it\x07\x1b[31mred\x1b[0m text\r\nline2\x1b[2Cend\r\n\x1b[?25lDo you want to proceed? (y/n)"
	got := StripANSI(raw)
	if !strings.Contains(got, "red text") || !strings.Contains(got, "line2 end") {
		t.Fatalf("strip wrong: %q", got)
	}
	if parseOSCTitle([]byte(raw)) != "⠙ Working on it" || !titleSpinning("⠙ Working on it") || titleSpinning("✳ Claude Code") {
		t.Fatal("title spinner detection wrong")
	}
	if !matchNeedsInput(strings.ToLower(got)) {
		t.Fatal("should detect needs-input prompt")
	}
	if slugify("Fix Login Race!") != "fix-login-race" || !strings.HasPrefix(slugify("修复登录"), "task-") {
		t.Fatalf("slugify wrong: %q %q", slugify("Fix Login Race!"), slugify("修复登录"))
	}
}

// npm 装的 agent 在 Windows 上是 .cmd 垫片：必须经绝对路径的 cmd.exe 启动，
// 并且真的能在指定工作目录的伪终端里跑起来。
func TestCmdShimAgentLaunches(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	dir := t.TempDir()
	shim := filepath.Join(dir, "fakeagent.cmd")
	if err := os.WriteFile(shim, []byte("@echo off\r\necho shim-started-ok\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	argv := launchArgv(AgentPreset{ID: "fake", Command: "fakeagent", ResolvedPath: shim, FullAccessFlag: "--yolo"}, true)
	if !filepath.IsAbs(argv[0]) || argv[1] != "/c" || argv[2] != shim || argv[3] != "--yolo" {
		t.Fatalf("argv wrong: %v", argv)
	}
	s := newSession("t1", "tok", sessionSpec{Agent: "fake", Dir: t.TempDir(), Env: os.Environ()}, nil)
	if err := s.start(argv); err != nil {
		t.Fatalf("shim must start: %v", err)
	}
	waitFor(t, "shim output", 20*time.Second, func() bool { return strings.Contains(s.ReadText(20), "shim-started-ok") })
}

func TestUsageLimitWindows(t *testing.T) {
	line := []byte(`{"timestamp":"2026-11-16T14:53:43.039Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":1200}},"rate_limits":{"primary":{"used_percent":45.0,"window_minutes":43200,"resets_at":4102444800},"secondary":null}}}`)
	p, lim, ok := parseCodexLine(line, nil)
	if !ok || p.tokens != 1200 || lim["primary"].windowMin != 43200 || lim["primary"].percent != 45 {
		t.Fatalf("codex line parsed wrong: %+v %+v", p, lim)
	}
	if windowLabel(300) != "5h" || windowLabel(10080) != "7d" || windowLabel(43200) != "30d" {
		t.Fatal("window labels wrong")
	}
}

// ---------- worktree 全流程（真实 git） ----------

func TestWorktreeLifecycle(t *testing.T) {
	repo := gitInit(t)
	u := newTestUsecase(t, repo)
	ctx := context.Background()

	ws, err := u.ListWorkspaces(ctx, "", false)
	if err != nil || len(ws) != 1 || ws[0].Kind != KindPrimary || ws[0].Branch != "main" {
		t.Fatalf("primary workspace wrong: %+v %v", ws, err)
	}

	w, err := u.CreateWorkspace(ctx, CreateWorkspaceInput{ProjectID: "p1", Name: "Fix Login Race"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Branch != "fix-login-race" || w.Status != WorkspaceCreating {
		t.Fatalf("create returned %+v", w)
	}
	waitFor(t, "worktree ready", 30*time.Second, func() bool {
		cur, err := u.workspace(ctx, w.ID)
		return err == nil && cur.Status == WorkspaceReady
	})

	// 改一个已跟踪文件 + 新建一个未跟踪文件。
	if err := os.WriteFile(filepath.Join(w.Path, "app.go"), []byte("package app\n\nfunc A() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "notes.md"), []byte("# hi\nline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := u.Diff(ctx, w.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Files) != 2 || d.Additions != 3 || d.Deletions != 1 {
		t.Fatalf("diff wrong: %+v", d)
	}
	st, err := u.GitStatus(ctx, w.ID)
	if err != nil || st.Branch != "fix-login-race" || len(st.Files) != 2 {
		t.Fatalf("status wrong: %+v %v", st, err)
	}

	// 批注 → 拼提示。
	n, err := u.UpsertNote(ctx, Note{WorkspaceID: w.ID, Path: "app.go", Line: 3, Body: "返回值别写死", LineText: "func A() int { return 2 }"})
	if err != nil || n.ID == "" {
		t.Fatalf("upsert note: %v", err)
	}
	notes, _ := u.ListNotes(ctx, w.ID)
	if p := composeReviewPrompt(notes); !strings.Contains(p, "app.go 第 3 行") || !strings.Contains(p, "返回值别写死") {
		t.Fatalf("prompt wrong: %s", p)
	}

	// 暂存并提交。
	if out, err := u.Stage(ctx, w.ID, nil); err != nil {
		t.Fatalf("stage: %v %s", err, out)
	}
	if out, err := u.Commit(ctx, w.ID, "feat: change A", false); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	st, _ = u.GitStatus(ctx, w.ID)
	if len(st.Files) != 0 || st.Additions != 3 {
		t.Fatalf("after commit status wrong: %+v", st)
	}
	if _, err := u.Stage(ctx, w.ID, []string{"../evil"}); err == nil {
		t.Fatal("path traversal must be rejected")
	}

	// 分支有未合并提交：普通删除保留分支，强制删除才删。
	kept, msg, err := u.DeleteWorkspace(ctx, w.ID, false)
	if err != nil || !kept {
		t.Fatalf("expected branch kept: kept=%v msg=%s err=%v", kept, msg, err)
	}
	if !branchExists(ctx, repo, "fix-login-race") {
		t.Fatal("branch should be kept")
	}
	if _, err := os.Stat(w.Path); !os.IsNotExist(err) {
		t.Fatal("worktree dir should be removed")
	}
	_ = deleteBranch(ctx, repo, "fix-login-race", true)

	// 同名再建：分支名不冲突。
	w2, err := u.CreateWorkspace(ctx, CreateWorkspaceInput{ProjectID: "p1", Name: "Fix Login Race"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "second worktree ready", 30*time.Second, func() bool {
		cur, err := u.workspace(ctx, w2.ID)
		return err == nil && cur.Status == WorkspaceReady
	})
	kept, _, err = u.DeleteWorkspace(ctx, w2.ID, false)
	if err != nil || kept {
		t.Fatalf("clean branch should be deleted: kept=%v err=%v", kept, err)
	}
	events, _, _ := u.ListEvents(ctx, 10, false)
	if len(events) < 2 || events[0].Kind != EventWorktreeReady {
		t.Fatalf("events wrong: %+v", events)
	}
}

// ---------- 伪终端 + 编排命令 ----------

func TestShellTerminalAndOrchestrationCLI(t *testing.T) {
	repo := gitInit(t)
	u := newTestUsecase(t, repo)
	ctx := context.Background()
	primary, err := u.workspace(ctx, primaryID("p1"))
	if err != nil {
		t.Fatal(err)
	}

	info, err := u.CreateTerminal(ctx, CreateTerminalInput{WorkspaceID: primary.ID})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if info.Status != StatusShell {
		t.Fatalf("shell status = %s", info.Status)
	}
	s, _ := u.Session(info.ID)
	waitFor(t, "shell prompt", 20*time.Second, func() bool { return s.Info().LastActivityAt.After(time.Time{}) })
	if err := s.Write([]byte("echo fleet-$((40+2))-ok\r")); err != nil {
		t.Fatal(err)
	}
	if err := s.Write([]byte("echo fleet-ps-ok\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "echo output", 20*time.Second, func() bool {
		txt := s.ReadText(40)
		return strings.Contains(txt, "fleet-42-ok") || strings.Contains(txt, "fleet-ps-ok\n") || strings.Count(txt, "fleet-ps-ok") >= 2
	})
	// 进程自然退出 → 状态落 exited；重启后换了新进程。
	oldPID := s.Info().PID
	_ = s.Write([]byte("exit\r"))
	waitFor(t, "shell exit", 20*time.Second, func() bool {
		st := s.Info().Status
		return st == StatusExited || st == StatusFailed
	})
	if _, err := u.RestartTerminal(info.ID); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if got := s.Info(); got.Status != StatusShell || got.PID == oldPID {
		t.Fatalf("restart did not spawn a new process: %+v", got)
	}
	if err := u.CloseTerminal(info.ID); err != nil {
		t.Fatal(err)
	}
	if len(u.ListTerminals("")) != 0 {
		t.Fatal("terminal should be gone after close")
	}

	// 编排：用 shell 冒充协调者与 worker，走 fleetctl 的服务端逻辑。
	run := Run{ID: "run_test", Objective: "demo", ProjectID: "p1", WorkspaceID: primary.ID, Agent: "shell", Status: RunRunning, CreatedAt: nowString()}
	coord, err := u.startTerminal(primary, sessionSpec{WorkspaceID: primary.ID, Agent: "shell", RunID: run.ID, Role: "coordinator"})
	if err != nil {
		t.Fatal(err)
	}
	run.CoordinatorTerminalID = coord.ID
	_ = putDoc(ctx, u.store, kindRun, run.ID, run)
	cs, _ := u.Session(coord.ID)

	if _, err := u.HandleCLI(ctx, "bad-token", "status", nil); err == nil {
		t.Fatal("bad token must be rejected")
	}
	out, err := u.HandleCLI(ctx, cs.token, "task-create", map[string]string{"title": "补单测", "spec": "给 app 补测试"})
	if err != nil || !strings.Contains(out, "已创建任务") {
		t.Fatalf("task-create: %s %v", out, err)
	}
	d, _ := u.GetRun(ctx, run.ID)
	if len(d.Tasks) != 1 || d.Tasks[0].Status != TaskReady {
		t.Fatalf("task not created: %+v", d.Tasks)
	}
	task := d.Tasks[0]
	if _, err := u.HandleCLI(ctx, cs.token, "worker-start", map[string]string{"task": task.ID, "agent": "shell"}); err == nil {
		t.Fatal("shell must not be accepted as worker agent")
	}

	worker, err := u.startTerminal(primary, sessionSpec{WorkspaceID: primary.ID, Agent: "shell", RunID: run.ID, TaskID: task.ID, Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	task.Status, task.WorkerTerminalID = TaskDispatched, worker.ID
	_ = putDoc(ctx, u.store, kindTask, task.ID, task)
	ws, _ := u.Session(worker.ID)
	if _, err := u.HandleCLI(ctx, ws.token, "task-create", map[string]string{"spec": "x"}); err == nil {
		t.Fatal("worker must not create tasks")
	}
	out, err = u.HandleCLI(ctx, ws.token, "report", map[string]string{"outcome": "succeeded", "body": "加了 3 个用例", "files": "app_test.go"})
	if err != nil || !strings.Contains(out, "已汇报") {
		t.Fatalf("report: %s %v", out, err)
	}
	out, err = u.HandleCLI(ctx, cs.token, "check", nil)
	if err != nil || !strings.Contains(out, "worker_done") || !strings.Contains(out, "加了 3 个用例") {
		t.Fatalf("check: %s %v", out, err)
	}
	if out, _ := u.HandleCLI(ctx, cs.token, "check", nil); out != "没有新消息。" {
		t.Fatalf("second check should be empty: %s", out)
	}
	d, _ = u.GetRun(ctx, run.ID)
	if d.Tasks[0].Status != TaskCompleted || len(d.Tasks[0].FilesModified) != 1 {
		t.Fatalf("task not completed: %+v", d.Tasks[0])
	}
	if out, err := u.HandleCLI(ctx, cs.token, "done", map[string]string{"summary": "ok"}); err != nil || !strings.Contains(out, "完成") {
		t.Fatalf("done: %s %v", out, err)
	}
}
