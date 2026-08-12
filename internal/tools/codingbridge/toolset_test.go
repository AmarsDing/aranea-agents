package codingbridge

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz/agentbridge"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// fakeBridgeService 记录工具调用并返回罐头结果。
type fakeBridgeService struct {
	dispatchRes *agentbridge.DispatchResult
	dispatchErr error
	gotSession  string
	gotAgentKey string
	gotProject  string
	gotPrompt   string

	task    *agentbridge.CodingTask
	taskErr error

	tasks []*agentbridge.CodingTask

	cancelledID string
	cancelErr   error
}

func (f *fakeBridgeService) DispatchTask(_ context.Context, sessionID, agentKey, projectQuery, prompt string) (*agentbridge.DispatchResult, error) {
	f.gotSession, f.gotAgentKey, f.gotProject, f.gotPrompt = sessionID, agentKey, projectQuery, prompt
	return f.dispatchRes, f.dispatchErr
}

func (f *fakeBridgeService) GetTask(_ context.Context, id string) (*agentbridge.CodingTask, error) {
	if f.task != nil && f.task.ID == id {
		return f.task, nil
	}
	if f.taskErr != nil {
		return nil, f.taskErr
	}
	return nil, errors.New("task not found")
}

func (f *fakeBridgeService) ListSessionTasks(_ context.Context, sessionID string, _ int) ([]*agentbridge.CodingTask, error) {
	var out []*agentbridge.CodingTask
	for _, t := range f.tasks {
		if t.SessionID == sessionID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeBridgeService) CancelTask(_ context.Context, id string) error {
	f.cancelledID = id
	return f.cancelErr
}

func invocationCtx(sessionID string) context.Context {
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{AppName: "app", UserID: "u1", ID: sessionID},
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func toolByName(t *testing.T, ts *ToolSet, name string) interface {
	Call(context.Context, []byte) (any, error)
} {
	t.Helper()
	for _, tl := range ts.Tools(context.Background()) {
		if tl.Declaration().Name == name {
			return tl.(interface {
				Call(context.Context, []byte) (any, error)
			})
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func asMap(t *testing.T, res any) map[string]any {
	t.Helper()
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map envelope", res)
	}
	return m
}

func TestToolSet_NameAndMembers(t *testing.T) {
	ts := NewToolSet(&fakeBridgeService{})
	if ts.Name() != "coding" {
		t.Fatalf("toolset name = %q, want coding", ts.Name())
	}
	tools := ts.Tools(context.Background())
	if len(tools) != 3 {
		t.Fatalf("tools = %d, want 3 (dispatch/check/cancel)", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Declaration().Name] = true
	}
	for _, want := range []string{"dispatch_task", "check_task", "cancel_task"} {
		if !names[want] {
			t.Fatalf("member names = %v, missing %s", names, want)
		}
	}
}

func TestDispatchTask_Success(t *testing.T) {
	fake := &fakeBridgeService{dispatchRes: &agentbridge.DispatchResult{
		Task: &agentbridge.CodingTask{ID: "task-1", Status: agentbridge.StatusRunning},
	}}
	ts := NewToolSet(fake)
	res, err := toolByName(t, ts, "dispatch_task").Call(
		invocationCtx("sess-1"),
		[]byte(`{"agent_key":"codebuddy","project_name":"aranea","task":"修复样式"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if fake.gotSession != "sess-1" || fake.gotAgentKey != "codebuddy" ||
		fake.gotProject != "aranea" || fake.gotPrompt != "修复样式" {
		t.Fatalf("dispatch args = %q/%q/%q/%q", fake.gotSession, fake.gotAgentKey, fake.gotProject, fake.gotPrompt)
	}
	m := asMap(t, res)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("result = %v, want ok=true", m)
	}
	if m["task_id"] != "task-1" || m["status"] != string(agentbridge.StatusRunning) {
		t.Fatalf("task_id/status = %v/%v", m["task_id"], m["status"])
	}
}

func TestDispatchTask_AmbiguousReturnsCandidates(t *testing.T) {
	fake := &fakeBridgeService{dispatchRes: &agentbridge.DispatchResult{
		Candidates: []agentbridge.ProjectCandidate{
			{ID: "p1", Name: "aranea-agents", Path: `F:\aranea-agents`},
			{ID: "p2", Name: "aranea-web", Path: `F:\aranea-web`},
		},
	}}
	ts := NewToolSet(fake)
	res, err := toolByName(t, ts, "dispatch_task").Call(
		invocationCtx("sess-1"),
		[]byte(`{"agent_key":"codebuddy","project_name":"aranea","task":"x"}`))
	if err != nil {
		t.Fatalf("ambiguous dispatch must not be Go error, got %v", err)
	}
	m := asMap(t, res)
	if ok, _ := m["ok"].(bool); ok {
		t.Fatalf("result = %v, want ok=false", m)
	}
	if code, _ := m["error_code"].(string); code != "AMBIGUOUS_PROJECT" {
		t.Fatalf("error_code = %v, want AMBIGUOUS_PROJECT", m["error_code"])
	}
	cands, ok := m["candidates"].([]map[string]any)
	if !ok || len(cands) != 2 {
		t.Fatalf("candidates = %v, want 2 entries", m["candidates"])
	}
	if cands[0]["name"] != "aranea-agents" || cands[1]["name"] != "aranea-web" {
		t.Fatalf("candidate names = %v/%v", cands[0]["name"], cands[1]["name"])
	}
}

func TestDispatchTask_MissingArgsStructuredError(t *testing.T) {
	ts := NewToolSet(&fakeBridgeService{})
	res, err := toolByName(t, ts, "dispatch_task").Call(
		invocationCtx("sess-1"), []byte(`{"agent_key":"codebuddy"}`))
	if err != nil {
		t.Fatalf("input validation must be structured envelope, got Go error %v", err)
	}
	m := asMap(t, res)
	if ok, _ := m["ok"].(bool); ok {
		t.Fatalf("result = %v, want ok=false", m)
	}
	if code, _ := m["error_code"].(string); code != "INVALID_ARGS" {
		t.Fatalf("error_code = %v, want INVALID_ARGS", m["error_code"])
	}
}

func TestDispatchTask_ServiceErrorStructured(t *testing.T) {
	fake := &fakeBridgeService{dispatchErr: errors.New("agent ghost is disabled")}
	ts := NewToolSet(fake)
	res, err := toolByName(t, ts, "dispatch_task").Call(
		invocationCtx("sess-1"),
		[]byte(`{"agent_key":"ghost","project_name":"aranea","task":"x"}`))
	if err != nil {
		t.Fatalf("service error must surface as envelope, got Go error %v", err)
	}
	m := asMap(t, res)
	if ok, _ := m["ok"].(bool); ok {
		t.Fatalf("result = %v, want ok=false", m)
	}
	if e, _ := m["error"].(string); e == "" {
		t.Fatal("error message required")
	}
}

func TestCheckTask_ByID(t *testing.T) {
	fake := &fakeBridgeService{task: &agentbridge.CodingTask{
		ID: "task-9", SessionID: "sess-1", Status: agentbridge.StatusDone,
		Summary: "修复完成", ProgressCount: 7,
	}}
	ts := NewToolSet(fake)
	res, err := toolByName(t, ts, "check_task").Call(invocationCtx("sess-1"), []byte(`{"task_id":"task-9"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	m := asMap(t, res)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("result = %v, want ok=true", m)
	}
	if m["task_id"] != "task-9" || m["status"] != string(agentbridge.StatusDone) {
		t.Fatalf("task_id/status = %v/%v", m["task_id"], m["status"])
	}
	if m["summary"] != "修复完成" {
		t.Fatalf("summary = %v", m["summary"])
	}
	if pc, _ := m["progress_count"].(int); pc != 7 {
		t.Fatalf("progress_count = %v, want 7", m["progress_count"])
	}
}

func TestCheckTask_DefaultsToLatestSessionTask(t *testing.T) {
	fake := &fakeBridgeService{tasks: []*agentbridge.CodingTask{
		{ID: "task-latest", SessionID: "sess-1", Status: agentbridge.StatusRunning},
		{ID: "task-other", SessionID: "sess-2", Status: agentbridge.StatusDone},
	}}
	ts := NewToolSet(fake)
	res, err := toolByName(t, ts, "check_task").Call(invocationCtx("sess-1"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	m := asMap(t, res)
	if m["task_id"] != "task-latest" {
		t.Fatalf("task_id = %v, want latest session task", m["task_id"])
	}
}

func TestCheckTask_NoTaskStructuredError(t *testing.T) {
	ts := NewToolSet(&fakeBridgeService{})
	res, err := toolByName(t, ts, "check_task").Call(invocationCtx("sess-1"), []byte(`{}`))
	if err != nil {
		t.Fatalf("no-task must be structured envelope, got Go error %v", err)
	}
	m := asMap(t, res)
	if ok, _ := m["ok"].(bool); ok {
		t.Fatalf("result = %v, want ok=false", m)
	}
	if code, _ := m["error_code"].(string); code != "NO_TASK" {
		t.Fatalf("error_code = %v, want NO_TASK", m["error_code"])
	}
}

func TestCancelTask_ByID(t *testing.T) {
	fake := &fakeBridgeService{}
	ts := NewToolSet(fake)
	res, err := toolByName(t, ts, "cancel_task").Call(invocationCtx("sess-1"), []byte(`{"task_id":"task-3"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if fake.cancelledID != "task-3" {
		t.Fatalf("cancelled id = %q, want task-3", fake.cancelledID)
	}
	m := asMap(t, res)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("result = %v, want ok=true", m)
	}
}

func TestCancelTask_DefaultsToLatestActive(t *testing.T) {
	fake := &fakeBridgeService{tasks: []*agentbridge.CodingTask{
		{ID: "task-done", SessionID: "sess-1", Status: agentbridge.StatusDone},
		{ID: "task-active", SessionID: "sess-1", Status: agentbridge.StatusRunning},
	}}
	ts := NewToolSet(fake)
	res, err := toolByName(t, ts, "cancel_task").Call(invocationCtx("sess-1"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if fake.cancelledID != "task-active" {
		t.Fatalf("cancelled id = %q, want task-active", fake.cancelledID)
	}
	m := asMap(t, res)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("result = %v, want ok=true", m)
	}
}

func TestCancelTask_NoActiveStructuredError(t *testing.T) {
	fake := &fakeBridgeService{tasks: []*agentbridge.CodingTask{
		{ID: "task-done", SessionID: "sess-1", Status: agentbridge.StatusDone},
	}}
	ts := NewToolSet(fake)
	res, err := toolByName(t, ts, "cancel_task").Call(invocationCtx("sess-1"), []byte(`{}`))
	if err != nil {
		t.Fatalf("no-active must be structured envelope, got Go error %v", err)
	}
	m := asMap(t, res)
	if code, _ := m["error_code"].(string); code != "NO_ACTIVE_TASK" {
		t.Fatalf("error_code = %v, want NO_ACTIVE_TASK", m["error_code"])
	}
}

func TestTool_NoInvocationContext(t *testing.T) {
	ts := NewToolSet(&fakeBridgeService{})
	for _, name := range []string{"dispatch_task", "check_task", "cancel_task"} {
		if _, err := toolByName(t, ts, name).Call(context.Background(), []byte(`{}`)); err == nil {
			t.Fatalf("%s: expected error when invocation context is absent", name)
		}
	}
}
