package agentbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
)

// ---------- 内存 mock ----------

type mockAgentRepo struct {
	byKey map[string]*CodingAgent
}

func (m *mockAgentRepo) GetByKey(_ context.Context, workspace, key string) (*CodingAgent, error) {
	a, ok := m.byKey[workspace+"/"+key]
	if !ok {
		return nil, errTestNotFound
	}
	cp := *a
	return &cp, nil
}

func (m *mockAgentRepo) List(_ context.Context, workspace string) ([]*CodingAgent, error) {
	var out []*CodingAgent
	for k, a := range m.byKey {
		if k[:len(workspace)+1] == workspace+"/" {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *mockAgentRepo) Upsert(_ context.Context, agent *CodingAgent) error {
	if m.byKey == nil {
		m.byKey = map[string]*CodingAgent{}
	}
	cp := *agent
	m.byKey[agent.Workspace+"/"+agent.AgentKey] = &cp
	return nil
}

func (m *mockAgentRepo) UpdateProbe(_ context.Context, id string, ok bool, errMsg string) error {
	return nil
}

type mockProjectRepo struct {
	byName map[string]*CodingProject
}

func (m *mockProjectRepo) GetByName(_ context.Context, workspace, name string) (*CodingProject, error) {
	p, ok := m.byName[workspace+"/"+name]
	if !ok {
		return nil, errTestNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *mockProjectRepo) Match(_ context.Context, workspace, query string) ([]*CodingProject, error) {
	var out []*CodingProject
	for k, p := range m.byName {
		if k[:len(workspace)+1] != workspace+"/" {
			continue
		}
		if containsFold(p.Name, query) {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *mockProjectRepo) List(_ context.Context, workspace string) ([]*CodingProject, error) {
	var out []*CodingProject
	for k, p := range m.byName {
		if k[:len(workspace)+1] == workspace+"/" {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *mockProjectRepo) Upsert(_ context.Context, p *CodingProject) error { return nil }
func (m *mockProjectRepo) Delete(_ context.Context, id string) error        { return nil }

func containsFold(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexFold(s, sub) >= 0)
}

func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFoldAt(s, i, sub) {
			return i
		}
	}
	return -1
}

func equalFoldAt(s string, off int, sub string) bool {
	for i := 0; i < len(sub); i++ {
		a, b := s[off+i], sub[i]
		if a >= 'A' && a <= 'Z' {
			a += 32
		}
		if b >= 'A' && b <= 'Z' {
			b += 32
		}
		if a != b {
			return false
		}
	}
	return true
}

type mockTaskRepo struct {
	mu   sync.Mutex
	byID map[string]*CodingTask
	seq  int
}

func newMockTaskRepo() *mockTaskRepo { return &mockTaskRepo{byID: map[string]*CodingTask{}} }

func (m *mockTaskRepo) Create(_ context.Context, t *CodingTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	t.ID = fmt.Sprintf("task-%d", m.seq)
	t.CreatedAt = fmt.Sprintf("2026-08-12T00:00:%02dZ", m.seq)
	t.UpdatedAt = t.CreatedAt
	cp := *t
	m.byID[t.ID] = &cp
	return nil
}

func (m *mockTaskRepo) Get(_ context.Context, id string) (*CodingTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byID[id]
	if !ok {
		return nil, errTestNotFound
	}
	cp := *t
	return &cp, nil
}

func (m *mockTaskRepo) UpdateStatus(_ context.Context, id string, from, to TaskStatus, patch TaskPatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byID[id]
	if !ok {
		return errTestNotFound
	}
	if t.Status != from {
		return errTestConflict
	}
	t.Status = to
	if patch.ACPSessionID != nil {
		t.ACPSessionID = *patch.ACPSessionID
	}
	if patch.Summary != nil {
		t.Summary = *patch.Summary
	}
	if patch.Error != nil {
		t.Error = *patch.Error
	}
	if patch.CompletedAt != nil {
		t.CompletedAt = *patch.CompletedAt
	}
	if patch.ProgressCount != nil {
		t.ProgressCount = *patch.ProgressCount
	}
	return nil
}

func (m *mockTaskRepo) ListBySession(_ context.Context, sessionID string, limit int) ([]*CodingTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*CodingTask
	for _, t := range m.byID {
		if t.SessionID == sessionID {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *mockTaskRepo) ListActive(_ context.Context) ([]*CodingTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*CodingTask
	for _, t := range m.byID {
		if !t.Status.IsTerminal() {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

var errTestNotFound = apierror.NotFound(apierror.DomainAgentBridge, "not found")
var errTestConflict = apierror.Conflict(apierror.DomainAgentBridge, "conflict")

// mockListener 记录终态通知（TaskListener）。
type mockListener struct {
	mu    sync.Mutex
	tasks []*CodingTask
}

func (l *mockListener) OnTaskTerminal(t *CodingTask) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := *t
	l.tasks = append(l.tasks, &cp)
}

func (l *mockListener) waitN(t *testing.T, n int) []*CodingTask {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		if len(l.tasks) >= n {
			out := l.tasks
			l.mu.Unlock()
			return out
		}
		l.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("listener notifications < %d", n)
	return nil
}

// ---------- ACP mock ----------

type mockACPSession struct {
	mu       sync.Mutex
	promptFn func(ctx context.Context, cwd, prompt string, h EventHandler) (string, error)
	// cancelCh 模拟真实 ACPSession.Cancel 中断进行中的 Prompt。
	cancelCh chan struct{}
	closed   bool
}

func (s *mockACPSession) Prompt(ctx context.Context, cwd, prompt string, h EventHandler) (string, error) {
	return s.promptFn(ctx, cwd, prompt, h)
}

func (s *mockACPSession) Cancel(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelCh != nil {
		close(s.cancelCh)
	}
	return nil
}

func (s *mockACPSession) Close() error { s.closed = true; return nil }

type mockSessionFactory struct {
	mu       sync.Mutex
	sessions []*mockACPSession
	spawnErr error
}

func (f *mockSessionFactory) Spawn(_ context.Context, agent *CodingAgent) (ACPSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.spawnErr != nil {
		return nil, f.spawnErr
	}
	if len(f.sessions) == 0 {
		return nil, errors.New("no mock session left")
	}
	s := f.sessions[0]
	f.sessions = f.sessions[1:]
	return s, nil
}

// ---------- 测试辅助 ----------

type testEnv struct {
	agents   *mockAgentRepo
	projects *mockProjectRepo
	tasks    *mockTaskRepo
	factory  *mockSessionFactory
	uc       *AgentBridgeUsecase
}

func newTestEnv() *testEnv {
	e := &testEnv{
		agents:   &mockAgentRepo{byKey: map[string]*CodingAgent{}},
		projects: &mockProjectRepo{byName: map[string]*CodingProject{}},
		tasks:    newMockTaskRepo(),
		factory:  &mockSessionFactory{},
	}
	e.uc = NewAgentBridgeUsecase(UsecaseDeps{
		Agents:   e.agents,
		Projects: e.projects,
		Tasks:    e.tasks,
		Sessions: e.factory,
	})
	return e
}

func (e *testEnv) seedAgent(key string, enabled bool) {
	e.agents.byKey["default/"+key] = &CodingAgent{
		ID: "agent-" + key, Workspace: "default", AgentKey: key,
		DisplayName: key, Command: key, Enabled: enabled,
	}
}

func (e *testEnv) seedProject(name, path string) {
	e.projects.byName["default/"+name] = &CodingProject{
		ID: "proj-" + name, Workspace: "default", Name: name, Path: path,
	}
}

func waitStatus(t *testing.T, repo *mockTaskRepo, id string, want TaskStatus) *CodingTask {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := repo.Get(context.Background(), id)
		if err == nil && task.Status == want {
			return task
		}
		time.Sleep(5 * time.Millisecond)
	}
	task, _ := repo.Get(context.Background(), id)
	t.Fatalf("task %s did not reach %s (current=%v)", id, want, task)
	return nil
}

func noopHandler() EventHandler { return discardHandler{} }

type discardHandler struct{}

func (discardHandler) OnUpdate(_, _ string) {}
func (discardHandler) OnPermission(_ context.Context, _ string, opts []PermissionOption) (string, error) {
	if len(opts) == 0 {
		return "", errors.New("no options")
	}
	return opts[0].OptionID, nil
}

// ---------- Dispatch 测试 ----------

func TestUsecase_DispatchAgentNotFound(t *testing.T) {
	e := newTestEnv()
	e.seedProject("proj", `F:\proj`)
	_, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "ghost", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestUsecase_DispatchAgentDisabled(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", false)
	e.seedProject("proj", `F:\proj`)
	_, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err == nil {
		t.Fatal("expected error for disabled agent")
	}
}

func TestUsecase_DispatchProjectZeroCandidates(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("aranea-agents", `F:\aranea-agents`)
	_, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "nothing", Prompt: "x", Handler: noopHandler(),
	})
	if err == nil {
		t.Fatal("expected error for zero candidates")
	}
}

func TestUsecase_DispatchProjectMultipleCandidates(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("aranea-agents", `F:\aranea-agents`)
	e.seedProject("aranea-web", `F:\aranea-web`)
	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "aranea", Prompt: "x", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("multiple candidates must not error, got %v", err)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(res.Candidates))
	}
	if res.Task != nil {
		t.Fatal("no task must be persisted when ambiguous")
	}
	if n := len(e.tasks.byID); n != 0 {
		t.Fatalf("tasks persisted = %d, want 0", n)
	}
}

func TestUsecase_DispatchSuccessRunsToDone(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("aranea-agents", `F:\aranea-agents`)

	sess := &mockACPSession{
		promptFn: func(ctx context.Context, cwd, prompt string, h EventHandler) (string, error) {
			if cwd != `F:\aranea-agents` {
				t.Errorf("cwd = %q", cwd)
			}
			h.OnUpdate("agent_message_chunk", "工作中")
			return "完成了 3 处修改", nil
		},
	}
	e.factory.sessions = []*mockACPSession{sess}

	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "aranea-agents",
		Prompt: "修复样式", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.Task == nil || res.Task.Status != StatusRunning {
		t.Fatalf("task = %+v, want running", res.Task)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("unexpected candidates: %+v", res.Candidates)
	}

	done := waitStatus(t, e.tasks, res.Task.ID, StatusDone)
	if done.Summary != "完成了 3 处修改" {
		t.Fatalf("summary = %q", done.Summary)
	}
	if done.CompletedAt == "" {
		t.Fatal("completed_at must be set")
	}
}

func TestUsecase_DispatchSpawnFailMarksTaskFailed(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)
	e.factory.spawnErr = errors.New("spawn: executable file not found in %PATH%")

	_, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err == nil {
		t.Fatal("expected spawn error")
	}
	// 任务已落库为 failed（无僵死 dispatched）。
	for _, tk := range e.tasks.byID {
		if tk.Status != StatusFailed {
			t.Fatalf("task status = %s, want failed", tk.Status)
		}
		if tk.Error == "" {
			t.Fatal("error must be recorded")
		}
	}
}

func TestUsecase_DispatchPromptErrorMarksFailed(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)
	e.factory.sessions = []*mockACPSession{{
		promptFn: func(ctx context.Context, _, _ string, _ EventHandler) (string, error) {
			return "", errors.New("agent crashed: exit status 1")
		},
	}}

	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	failed := waitStatus(t, e.tasks, res.Task.ID, StatusFailed)
	if failed.Error == "" {
		t.Fatal("error must be recorded")
	}
}

func TestUsecase_DispatchConcurrencyLimit(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)

	// 两个同 agent 活跃任务（running）。
	for i := 0; i < 2; i++ {
		tk := &CodingTask{
			Workspace: "default", SessionID: "s1", AgentID: "agent-codebuddy",
			ProjectID: "proj-proj", Prompt: "x", Status: StatusRunning,
		}
		if err := e.tasks.Create(context.Background(), tk); err != nil {
			t.Fatal(err)
		}
	}
	e.factory.sessions = []*mockACPSession{{
		promptFn: func(ctx context.Context, _, _ string, _ EventHandler) (string, error) { return "ok", nil },
	}}

	_, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err == nil {
		t.Fatal("expected concurrency limit error")
	}
}

// ---------- Cancel 测试 ----------

func TestUsecase_CancelRunningTask(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)

	promptBlocked := make(chan struct{})
	cancelCh := make(chan struct{})
	sess := &mockACPSession{
		cancelCh: cancelCh,
		promptFn: func(ctx context.Context, _, _ string, _ EventHandler) (string, error) {
			close(promptBlocked)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-cancelCh: // 真实实现：Cancel 中断进行中的 Prompt
				return "", context.Canceled
			}
		},
	}
	e.factory.sessions = []*mockACPSession{sess}

	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	<-promptBlocked // 确认 Prompt 已在执行

	if err := e.uc.Cancel(context.Background(), res.Task.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	cancelled := waitStatus(t, e.tasks, res.Task.ID, StatusCancelled)
	if cancelled.CompletedAt == "" {
		t.Fatal("completed_at must be set on cancel")
	}
}

func TestUsecase_CancelTerminalTaskConflicts(t *testing.T) {
	e := newTestEnv()
	tk := &CodingTask{
		Workspace: "default", SessionID: "s", AgentID: "a", ProjectID: "p",
		Prompt: "x", Status: StatusDone,
	}
	if err := e.tasks.Create(context.Background(), tk); err != nil {
		t.Fatal(err)
	}
	if err := e.uc.Cancel(context.Background(), tk.ID); err == nil {
		t.Fatal("expected conflict cancelling terminal task")
	}
}

// ---------- 终态监听（TaskListener） ----------

func TestUsecase_ListenerNotifiedOnDone(t *testing.T) {
	e := newTestEnv()
	lis := &mockListener{}
	e.uc = NewAgentBridgeUsecase(UsecaseDeps{
		Agents: e.agents, Projects: e.projects, Tasks: e.tasks,
		Sessions: e.factory, Listener: lis,
	})
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)
	e.factory.sessions = []*mockACPSession{{
		promptFn: func(_ context.Context, _, _ string, h EventHandler) (string, error) {
			h.OnUpdate("agent_message_chunk", "a")
			h.OnUpdate("agent_message_chunk", "b")
			return "done-summary", nil
		},
	}}

	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got := lis.waitN(t, 1)[0]
	if got.ID != res.Task.ID || got.Status != StatusDone {
		t.Fatalf("notified task = %s/%s, want %s/done", got.ID, got.Status, res.Task.ID)
	}
	if got.Summary != "done-summary" {
		t.Fatalf("summary = %q", got.Summary)
	}
	if got.ProgressCount != 2 {
		t.Fatalf("progress_count = %d, want 2", got.ProgressCount)
	}
}

func TestUsecase_ListenerNotifiedOnFailureAndCancel(t *testing.T) {
	e := newTestEnv()
	lis := &mockListener{}
	e.uc = NewAgentBridgeUsecase(UsecaseDeps{
		Agents: e.agents, Projects: e.projects, Tasks: e.tasks,
		Sessions: e.factory, Listener: lis,
	})
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)

	// 失败路径
	e.factory.sessions = []*mockACPSession{{
		promptFn: func(context.Context, string, string, EventHandler) (string, error) {
			return "", errors.New("boom")
		},
	}}
	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	_ = res
	got := lis.waitN(t, 1)[0]
	if got.Status != StatusFailed || got.Error == "" {
		t.Fatalf("notified = %s/%q, want failed with error", got.Status, got.Error)
	}

	// 取消路径
	cancelCh := make(chan struct{})
	blocked := make(chan struct{})
	e.factory.sessions = []*mockACPSession{{
		cancelCh: cancelCh,
		promptFn: func(ctx context.Context, _, _ string, _ EventHandler) (string, error) {
			close(blocked)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-cancelCh:
				return "", context.Canceled
			}
		},
	}}
	res2, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "y", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch2: %v", err)
	}
	<-blocked
	if err := e.uc.Cancel(context.Background(), res2.Task.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got2 := lis.waitN(t, 2)[1]
	if got2.ID != res2.Task.ID || got2.Status != StatusCancelled {
		t.Fatalf("notified = %s/%s, want %s/cancelled", got2.ID, got2.Status, res2.Task.ID)
	}
}

func TestUsecase_ListenerNotifiedOnSpawnFailure(t *testing.T) {
	e := newTestEnv()
	lis := &mockListener{}
	e.uc = NewAgentBridgeUsecase(UsecaseDeps{
		Agents: e.agents, Projects: e.projects, Tasks: e.tasks,
		Sessions: e.factory, Listener: lis,
	})
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)
	e.factory.spawnErr = errors.New("command not found")

	_, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err == nil {
		t.Fatal("expected spawn error")
	}
	got := lis.waitN(t, 1)[0]
	if got.Status != StatusFailed {
		t.Fatalf("notified = %s, want failed", got.Status)
	}
}

// TestUsecase_PromptPanicRecovered：run goroutine 内 Prompt panic 必须被 recover，
// 任务推进 failed 并通知 listener（K7：后台 goroutine panic 恢复必须留痕）。
func TestUsecase_PromptPanicRecovered(t *testing.T) {
	e := newTestEnv()
	lis := &mockListener{}
	e.uc = NewAgentBridgeUsecase(UsecaseDeps{
		Agents: e.agents, Projects: e.projects, Tasks: e.tasks,
		Sessions: e.factory, Listener: lis,
	})
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)
	e.factory.sessions = []*mockACPSession{{
		promptFn: func(context.Context, string, string, EventHandler) (string, error) {
			panic("acp conn exploded")
		},
	}}

	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got := lis.waitN(t, 1)[0]
	if got.ID != res.Task.ID || got.Status != StatusFailed {
		t.Fatalf("notified = %s/%s, want %s/failed", got.ID, got.Status, res.Task.ID)
	}
	if !strings.Contains(got.Error, "panic") {
		t.Fatalf("panic task error = %q, want contains panic", got.Error)
	}
}

// ---------- 启动恢复 ----------

func TestUsecase_RecoverActiveTasks(t *testing.T) {
	e := newTestEnv()
	for i, st := range []TaskStatus{StatusDispatched, StatusRunning, StatusAwaitingApproval, StatusCancelling, StatusDone} {
		tk := &CodingTask{
			Workspace: "default", SessionID: "s", AgentID: "a", ProjectID: "p",
			Prompt: fmt.Sprint(i), Status: st,
		}
		if err := e.tasks.Create(context.Background(), tk); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.uc.RecoverActiveTasks(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}
	active, err := e.tasks.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active after recover = %d, want 0", len(active))
	}
	for _, tk := range e.tasks.byID {
		if tk.Status == StatusDone {
			continue // 原终态不动
		}
		if tk.Status != StatusFailed || tk.Error == "" {
			t.Fatalf("task %s = %s/%q, want failed with reason", tk.ID, tk.Status, tk.Error)
		}
	}
}
