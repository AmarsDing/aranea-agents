package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/pkg/apierror"
)

// ---------- 内存 fake（窄接口实现） ----------

type abFakeAgents struct {
	byKey  map[string]*agentbridge.CodingAgent
	probes []abProbe
}

type abProbe struct {
	id     string
	ok     bool
	errMsg string
}

func (f *abFakeAgents) GetByKey(_ context.Context, workspace, key string) (*agentbridge.CodingAgent, error) {
	a, ok := f.byKey[workspace+"/"+key]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainAgentBridge, "agent not found")
	}
	cp := *a
	return &cp, nil
}

func (f *abFakeAgents) List(_ context.Context, workspace string) ([]*agentbridge.CodingAgent, error) {
	var out []*agentbridge.CodingAgent
	for k, a := range f.byKey {
		if k[:len(workspace)+1] == workspace+"/" {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *abFakeAgents) Upsert(_ context.Context, agent *agentbridge.CodingAgent) error {
	if f.byKey == nil {
		f.byKey = map[string]*agentbridge.CodingAgent{}
	}
	cp := *agent
	if cp.ID == "" {
		cp.ID = "agent-" + cp.AgentKey
	}
	f.byKey[agent.Workspace+"/"+agent.AgentKey] = &cp
	return nil
}

func (f *abFakeAgents) UpdateProbe(_ context.Context, id string, ok bool, errMsg string) error {
	f.probes = append(f.probes, abProbe{id: id, ok: ok, errMsg: errMsg})
	for _, a := range f.byKey {
		if a.ID == id {
			a.LastProbeOK = ok
			a.LastProbeError = errMsg
		}
	}
	return nil
}

type abFakeProjects struct {
	byName map[string]*agentbridge.CodingProject
}

func (f *abFakeProjects) GetByName(_ context.Context, workspace, name string) (*agentbridge.CodingProject, error) {
	p, ok := f.byName[workspace+"/"+name]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainAgentBridge, "project not found")
	}
	cp := *p
	return &cp, nil
}

func (f *abFakeProjects) Match(_ context.Context, workspace, query string) ([]*agentbridge.CodingProject, error) {
	var out []*agentbridge.CodingProject
	for k, p := range f.byName {
		if k[:len(workspace)+1] != workspace+"/" {
			continue
		}
		if len(query) > 0 && len(p.Name) >= len(query) && containsFoldAB(p.Name, query) {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

func containsFoldAB(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (f *abFakeProjects) List(_ context.Context, workspace string) ([]*agentbridge.CodingProject, error) {
	var out []*agentbridge.CodingProject
	for k, p := range f.byName {
		if k[:len(workspace)+1] == workspace+"/" {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *abFakeProjects) Upsert(_ context.Context, p *agentbridge.CodingProject) error {
	if f.byName == nil {
		f.byName = map[string]*agentbridge.CodingProject{}
	}
	cp := *p
	if cp.ID == "" {
		cp.ID = "proj-" + cp.Name
	}
	f.byName[p.Workspace+"/"+p.Name] = &cp
	return nil
}

func (f *abFakeProjects) Delete(_ context.Context, id string) error {
	for k, p := range f.byName {
		if p.ID == id {
			delete(f.byName, k)
		}
	}
	return nil
}

type abFakeTasks struct {
	mu   sync.Mutex
	byID map[string]*agentbridge.CodingTask
	seq  int
}

func newABFakeTasks() *abFakeTasks { return &abFakeTasks{byID: map[string]*agentbridge.CodingTask{}} }

func (f *abFakeTasks) Create(_ context.Context, t *agentbridge.CodingTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	t.ID = fmt.Sprintf("task-%d", f.seq)
	cp := *t
	f.byID[t.ID] = &cp
	return nil
}

func (f *abFakeTasks) Get(_ context.Context, id string) (*agentbridge.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainAgentBridge, "task not found")
	}
	cp := *t
	return &cp, nil
}

func (f *abFakeTasks) UpdateStatus(_ context.Context, id string, from, to agentbridge.TaskStatus, patch agentbridge.TaskPatch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return apierror.NotFound(apierror.DomainAgentBridge, "task not found")
	}
	if t.Status != from {
		return apierror.Conflict(apierror.DomainAgentBridge, "status conflict")
	}
	t.Status = to
	if patch.Summary != nil {
		t.Summary = *patch.Summary
	}
	if patch.Error != nil {
		t.Error = *patch.Error
	}
	if patch.CompletedAt != nil {
		t.CompletedAt = *patch.CompletedAt
	}
	if patch.ACPSessionID != nil {
		t.ACPSessionID = *patch.ACPSessionID
	}
	if patch.ProgressCount != nil {
		t.ProgressCount = *patch.ProgressCount
	}
	return nil
}

func (f *abFakeTasks) ListBySession(_ context.Context, sessionID string, limit int) ([]*agentbridge.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*agentbridge.CodingTask
	for _, t := range f.byID {
		if t.SessionID == sessionID {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *abFakeTasks) ListActive(_ context.Context) ([]*agentbridge.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*agentbridge.CodingTask
	for _, t := range f.byID {
		if !t.Status.IsTerminal() {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

type abFakeFactory struct {
	sessions []*abFakeSession
}

func (f *abFakeFactory) Spawn(_ context.Context, _ *agentbridge.CodingAgent) (agentbridge.ACPSession, error) {
	if len(f.sessions) == 0 {
		return nil, errors.New("no fake session left")
	}
	s := f.sessions[0]
	f.sessions = f.sessions[1:]
	return s, nil
}

type abFakeSession struct {
	promptFn func(ctx context.Context, cwd, prompt string, h agentbridge.EventHandler) (string, error)
}

func (s *abFakeSession) Prompt(ctx context.Context, cwd, prompt string, h agentbridge.EventHandler) (string, error) {
	return s.promptFn(ctx, cwd, prompt, h)
}
func (s *abFakeSession) Cancel(context.Context) error { return nil }
func (s *abFakeSession) Close() error                 { return nil }

type abFakeBus struct {
	mu     sync.Mutex
	events []biz.Event
}

func (b *abFakeBus) Publish(_ context.Context, e biz.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

func (b *abFakeBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return make(chan biz.Event), func() {}
}

func (b *abFakeBus) notices() []*biz.SystemNoticeEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*biz.SystemNoticeEvent
	for _, e := range b.events {
		if n, ok := e.(*biz.SystemNoticeEvent); ok {
			out = append(out, n)
		}
	}
	return out
}

func (b *abFakeBus) noticesByType(typ string) []*biz.SystemNoticeEvent {
	var out []*biz.SystemNoticeEvent
	for _, n := range b.notices() {
		if n.NoticeType == typ {
			out = append(out, n)
		}
	}
	return out
}

func (b *abFakeBus) waitNoticeType(t *testing.T, typ string, n int) []*biz.SystemNoticeEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := b.noticesByType(typ); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("notices[%s] < %d, got %v", typ, n, b.notices())
	return nil
}

// ---------- 测试环境 ----------

type abSvcEnv struct {
	agents   *abFakeAgents
	projects *abFakeProjects
	tasks    *abFakeTasks
	factory  *abFakeFactory
	bus      *abFakeBus
	clock    *abFakeClock
	svc      *AgentBridgeService
	api      *AgentBridgeAPI
}

type abFakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *abFakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *abFakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newABSvcEnv() *abSvcEnv {
	e := &abSvcEnv{
		agents:   &abFakeAgents{byKey: map[string]*agentbridge.CodingAgent{}},
		projects: &abFakeProjects{byName: map[string]*agentbridge.CodingProject{}},
		tasks:    newABFakeTasks(),
		factory:  &abFakeFactory{},
		bus:      &abFakeBus{},
		clock:    &abFakeClock{now: time.Now()},
	}
	e.svc = NewAgentBridgeService(AgentBridgeServiceDeps{
		Agents:   e.agents,
		Projects: e.projects,
		Bus:      e.bus,
		Clock:    e.clock.Now,
	})
	uc := agentbridge.NewAgentBridgeUsecase(agentbridge.UsecaseDeps{
		Agents:   e.agents,
		Projects: e.projects,
		Tasks:    e.tasks,
		Sessions: e.factory,
		Listener: e.svc,
	})
	e.svc.BindUsecase(uc)
	e.api = NewAgentBridgeAPI(e.svc)
	return e
}

func (e *abSvcEnv) seedAgent(key string) {
	e.agents.byKey["default/"+key] = &agentbridge.CodingAgent{
		ID: "agent-" + key, Workspace: "default", AgentKey: key,
		DisplayName: key + " 助手", Command: key, Enabled: true,
	}
}

func (e *abSvcEnv) seedProject(name, path string) {
	e.projects.byName["default/"+name] = &agentbridge.CodingProject{
		ID: "proj-" + name, Workspace: "default", Name: name, Path: path,
	}
}

// ---------- M1-8 测试：限流窗口与事件负载 ----------

func TestAgentBridgeService_ProgressThrottledByWindow(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codebuddy")
	e.seedProject("aranea", `F:\aranea`)

	emitted := make(chan struct{})
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(_ context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			h.OnUpdate("agent_message_chunk", "第一段")
			h.OnUpdate("agent_message_chunk", "第二段")
			h.OnUpdate("tool_call", "执行 go test")
			close(emitted)
			return "收尾", nil
		},
	}}

	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codebuddy", "aranea", "修复样式")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	<-emitted // 三次 OnUpdate 已调用

	// 窗口内三次更新 → 仅 1 条进度事件（leading edge）
	progress := e.bus.noticesByType("coding_task_progress")
	if len(progress) != 1 {
		t.Fatalf("progress events in window = %d, want 1", len(progress))
	}
	n := progress[0]
	if n.Meta["task_id"] != res.Task.ID {
		t.Fatalf("task_id = %v, want %s", n.Meta["task_id"], res.Task.ID)
	}
	if n.Meta["agent_key"] != "codebuddy" || n.Meta["project_name"] != "aranea" {
		t.Fatalf("meta agent/project = %v/%v", n.Meta["agent_key"], n.Meta["project_name"])
	}
	if n.Meta["session_id"] != "sess-1" {
		t.Fatalf("session_id = %v", n.Meta["session_id"])
	}
}

func TestAgentBridgeService_ProgressResumesAfterWindow(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codebuddy")
	e.seedProject("aranea", `F:\aranea`)

	second := make(chan struct{})
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(_ context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			h.OnUpdate("agent_message_chunk", "首条") // leading edge 立即发射
			e.clock.Advance(6 * time.Second)
			h.OnUpdate("agent_message_chunk", "窗口后") // 窗口已过 → 再发射
			close(second)
			return "ok", nil
		},
	}}

	_, err := e.svc.DispatchTask(context.Background(), "sess-1", "codebuddy", "aranea", "x")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	<-second
	progress := e.bus.noticesByType("coding_task_progress")
	if len(progress) != 2 {
		t.Fatalf("progress events = %d, want 2", len(progress))
	}
	if progress[1].Message == "" {
		t.Fatal("second event must carry latest text")
	}
}

func TestAgentBridgeService_TerminalCompletedNotice(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codebuddy")
	e.seedProject("aranea", `F:\aranea`)
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(context.Context, string, string, agentbridge.EventHandler) (string, error) {
			return "修复完成，改了 3 个文件", nil
		},
	}}

	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codebuddy", "aranea", "修复样式")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	notices := e.bus.waitNoticeType(t, "coding_task_completed", 1)
	n := notices[0]
	if n.Meta["task_id"] != res.Task.ID {
		t.Fatalf("task_id = %v", n.Meta["task_id"])
	}
	if n.Meta["summary"] != "修复完成，改了 3 个文件" {
		t.Fatalf("summary = %v", n.Meta["summary"])
	}
	if n.Meta["agent_key"] != "codebuddy" || n.Meta["project_name"] != "aranea" {
		t.Fatalf("agent/project = %v/%v", n.Meta["agent_key"], n.Meta["project_name"])
	}
	if n.SpiritSessionID() != "sess-1" {
		t.Fatalf("event session = %q", n.SpiritSessionID())
	}
}

func TestAgentBridgeService_TerminalFailedNotice(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codebuddy")
	e.seedProject("aranea", `F:\aranea`)
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(context.Context, string, string, agentbridge.EventHandler) (string, error) {
			return "", errors.New("agent crashed")
		},
	}}

	_, err := e.svc.DispatchTask(context.Background(), "sess-1", "codebuddy", "aranea", "x")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	notices := e.bus.waitNoticeType(t, "coding_task_failed", 1)
	n := notices[0]
	if n.Meta["error"] == nil || n.Meta["error"] == "" {
		t.Fatal("failed notice must carry error")
	}
}

func TestAgentBridgeService_DispatchCandidatesNoEvents(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codebuddy")
	e.seedProject("aranea-agents", `F:\aranea-agents`)
	e.seedProject("aranea-web", `F:\aranea-web`)

	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codebuddy", "aranea", "x")
	if err != nil {
		t.Fatalf("ambiguous dispatch must not error, got %v", err)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(res.Candidates))
	}
	if got := e.bus.notices(); len(got) != 0 {
		t.Fatalf("ambiguous dispatch must not emit events, got %d", len(got))
	}
}

func TestAgentBridgeService_ProbeAgent(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codebuddy")
	e.seedAgent("ghost")

	if err := e.svc.ProbeAgent(context.Background(), "default", "codebuddy"); err != nil {
		t.Fatalf("probe: %v", err)
	}
	// fake agent command = key（"codebuddy" 不存在于 PATH → 探测失败）
	if len(e.agents.probes) != 1 || e.agents.probes[0].ok {
		t.Fatalf("probe for nonexistent command must be ok=false: %+v", e.agents.probes)
	}
	if e.agents.probes[0].errMsg == "" {
		t.Fatal("failed probe must record error message")
	}

	// 存在的命令（go 必然在开发机 PATH）
	e.agents.byKey["default/go"] = &agentbridge.CodingAgent{
		ID: "agent-go", Workspace: "default", AgentKey: "go",
		DisplayName: "go", Command: "go", Enabled: true,
	}
	if err := e.svc.ProbeAgent(context.Background(), "default", "go"); err != nil {
		t.Fatalf("probe go: %v", err)
	}
	last := e.agents.probes[len(e.agents.probes)-1]
	if !last.ok {
		t.Fatalf("probe go must succeed: %+v", last)
	}
}
