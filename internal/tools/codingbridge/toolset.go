// Package codingbridge 把编程 Agent 桥接能力暴露为精灵工具：
// 派发/查询/取消外部编程 CLI（Claude Code / Codex / CodeBuddy）任务。
package codingbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"aranea-agents/internal/biz/agentbridge"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ToolSetName 是 codingbridge 组名（框架 NamedToolSet 前缀后得到
// coding_dispatch_task / coding_check_task / coding_cancel_task）。
const ToolSetName = "coding"

// 结构化错误码（LLM 可读的机器语义，设计 §7.4）。
const (
	errCodeInvalidArgs      = "INVALID_ARGS"
	errCodeAmbiguousProject = "AMBIGUOUS_PROJECT"
	errCodeNoTask           = "NO_TASK"
	errCodeNoActiveTask     = "NO_ACTIVE_TASK"
	errCodeDispatchFailed   = "DISPATCH_FAILED"
	errCodeCancelFailed     = "CANCEL_FAILED"
)

// BridgeService 是 codingbridge 对 service 层的窄依赖
// （*service.AgentBridgeService 实现；Wire 在 service 层装配）。
// Stability:internal
type BridgeService interface {
	DispatchTask(ctx context.Context, sessionID, agentKey, projectQuery, prompt string) (*agentbridge.DispatchResult, error)
	GetTask(ctx context.Context, id string) (*agentbridge.CodingTask, error)
	ListSessionTasks(ctx context.Context, sessionID string, limit int) ([]*agentbridge.CodingTask, error)
	CancelTask(ctx context.Context, id string) error
}

// ToolSet 暴露 coding_* 三工具。
type ToolSet struct {
	svc BridgeService
}

// NewToolSet 构造 ToolSet。svc 为 nil 时 Call 立即报错（装配顺序兜底）。
func NewToolSet(svc BridgeService) *ToolSet {
	return &ToolSet{svc: svc}
}

// Name implements trpctool.ToolSet.
func (s *ToolSet) Name() string { return ToolSetName }

// Close implements trpctool.ToolSet.
func (s *ToolSet) Close() error { return nil }

// Tools implements trpctool.ToolSet.
func (s *ToolSet) Tools(_ context.Context) []trpctool.Tool {
	return []trpctool.Tool{
		newDispatchTool(s.svc),
		newCheckTool(s.svc),
		newCancelTool(s.svc),
	}
}

// sessionIDFromCtx 从调用上下文取精灵会话 ID（工具调用必须挂在会话上）。
func sessionIDFromCtx(ctx context.Context) (string, error) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil || inv.Session.ID == "" {
		return "", errors.New("codingbridge: coding tools require an active session")
	}
	return inv.Session.ID, nil
}

func envelope(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		m[key] = kv[i+1]
	}
	return m
}

func errEnvelope(code, msg string) map[string]any {
	return envelope("ok", false, "error_code", code, "error", msg)
}

func taskEnvelope(t *agentbridge.CodingTask) map[string]any {
	m := envelope("ok", true, "task_id", t.ID, "status", string(t.Status),
		"progress_count", t.ProgressCount)
	if t.Summary != "" {
		m["summary"] = t.Summary
	}
	if t.Error != "" {
		m["error"] = t.Error
	}
	return m
}

// --- dispatch_task ---

type dispatchTool struct{ svc BridgeService }

func newDispatchTool(svc BridgeService) *dispatchTool { return &dispatchTool{svc: svc} }

func (t *dispatchTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "dispatch_task",
		Description: "Dispatch a coding task to an external coding agent (Claude Code / Codex / CodeBuddy) " +
			"running on a registered local project. Returns immediately with task_id; the task runs in background " +
			"and its result is announced when finished. If the project name is ambiguous, returns candidates " +
			"for the user to pick — then re-dispatch with the exact project name.",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"agent_key", "project_name", "task"},
			Properties: map[string]*trpctool.Schema{
				"agent_key":    {Type: "string", Description: "Registered coding agent key, e.g. claude_code / codex / codebuddy."},
				"project_name": {Type: "string", Description: "Registered project name (fuzzy match allowed; exact name avoids ambiguity)."},
				"task":         {Type: "string", Description: "The coding task description in natural language."},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Dispatch envelope.",
			Required:    []string{"ok"},
			Properties: map[string]*trpctool.Schema{
				"ok":          {Type: "boolean"},
				"task_id":     {Type: "string", Description: "Dispatched task id when ok is true."},
				"status":      {Type: "string", Description: "Task status after dispatch (running)."},
				"error":       {Type: "string"},
				"error_code":  {Type: "string", Description: "INVALID_ARGS / AMBIGUOUS_PROJECT / DISPATCH_FAILED."},
				"candidates":  {Type: "array", Description: "Project candidates when error_code is AMBIGUOUS_PROJECT."},
			},
		},
	}
}

type dispatchArgs struct {
	AgentKey    string `json:"agent_key"`
	ProjectName string `json:"project_name"`
	Task        string `json:"task"`
}

func (t *dispatchTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t.svc == nil {
		return nil, errors.New("codingbridge: toolset has no service")
	}
	sessionID, err := sessionIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	var args dispatchArgs
	if err := json.Unmarshal(jsonArgs, &args); err != nil {
		return errEnvelope(errCodeInvalidArgs, fmt.Sprintf("invalid JSON args: %v", err)), nil
	}
	if args.AgentKey == "" || args.ProjectName == "" || args.Task == "" {
		return errEnvelope(errCodeInvalidArgs, "agent_key, project_name and task are all required"), nil
	}
	res, err := t.svc.DispatchTask(ctx, sessionID, args.AgentKey, args.ProjectName, args.Task)
	if err != nil {
		return errEnvelope(errCodeDispatchFailed, err.Error()), nil
	}
	if res == nil {
		return errEnvelope(errCodeDispatchFailed, "dispatch returned no result"), nil
	}
	if len(res.Candidates) > 0 {
		cands := make([]map[string]any, 0, len(res.Candidates))
		for _, c := range res.Candidates {
			cands = append(cands, envelope("name", c.Name, "path", c.Path, "description", c.Description))
		}
		return envelope("ok", false, "error_code", errCodeAmbiguousProject,
			"error", "project name is ambiguous, ask the user to pick one and re-dispatch with the exact name",
			"candidates", cands), nil
	}
	if res.Task == nil {
		return errEnvelope(errCodeDispatchFailed, "dispatch returned no task"), nil
	}
	return envelope("ok", true, "task_id", res.Task.ID, "status", string(res.Task.Status)), nil
}

// --- check_task ---

type checkTool struct{ svc BridgeService }

func newCheckTool(svc BridgeService) *checkTool { return &checkTool{svc: svc} }

func (t *checkTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "check_task",
		Description: "Check the status and result of a coding task dispatched via dispatch_task. " +
			"Omit task_id to check the most recent coding task of the current session.",
		InputSchema: &trpctool.Schema{
			Type: "object",
			Properties: map[string]*trpctool.Schema{
				"task_id": {Type: "string", Description: "Task id returned by dispatch_task; omit for the latest task of this session."},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"ok"},
			Properties: map[string]*trpctool.Schema{
				"ok":             {Type: "boolean"},
				"task_id":        {Type: "string"},
				"status":         {Type: "string", Description: "dispatched/running/awaiting_approval/cancelling/done/failed/cancelled."},
				"summary":        {Type: "string", Description: "Result summary when status is done."},
				"error":          {Type: "string"},
				"error_code":     {Type: "string", Description: "NO_TASK when the session has no coding task."},
				"progress_count": {Type: "integer", Description: "Number of progress updates received from the agent."},
			},
		},
	}
}

func (t *checkTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t.svc == nil {
		return nil, errors.New("codingbridge: toolset has no service")
	}
	sessionID, err := sessionIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(jsonArgs, &args); err != nil {
		return errEnvelope(errCodeInvalidArgs, fmt.Sprintf("invalid JSON args: %v", err)), nil
	}
	task, err := t.resolveTask(ctx, sessionID, args.TaskID)
	if err != nil {
		return errEnvelope(errCodeNoTask, err.Error()), nil
	}
	return taskEnvelope(task), nil
}

// resolveTask 解析任务：显式 id 直查；缺省取本会话最近一个（ListBySession 倒序）。
func (t *checkTool) resolveTask(ctx context.Context, sessionID, taskID string) (*agentbridge.CodingTask, error) {
	if taskID != "" {
		return t.svc.GetTask(ctx, taskID)
	}
	tasks, err := t.svc.ListSessionTasks(ctx, sessionID, 1)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, errors.New("no coding task in this session")
	}
	return tasks[0], nil
}

// --- cancel_task ---

type cancelTool struct{ svc BridgeService }

func newCancelTool(svc BridgeService) *cancelTool { return &cancelTool{svc: svc} }

func (t *cancelTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "cancel_task",
		Description: "Cancel a running coding task dispatched via dispatch_task. " +
			"Omit task_id to cancel the most recent active (non-terminal) task of the current session.",
		InputSchema: &trpctool.Schema{
			Type: "object",
			Properties: map[string]*trpctool.Schema{
				"task_id": {Type: "string", Description: "Task id to cancel; omit for the latest active task of this session."},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"ok"},
			Properties: map[string]*trpctool.Schema{
				"ok":         {Type: "boolean"},
				"task_id":    {Type: "string"},
				"status":     {Type: "string", Description: "cancelling after a successful cancel request."},
				"error":      {Type: "string"},
				"error_code": {Type: "string", Description: "NO_ACTIVE_TASK / CANCEL_FAILED."},
			},
		},
	}
}

func (t *cancelTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t.svc == nil {
		return nil, errors.New("codingbridge: toolset has no service")
	}
	sessionID, err := sessionIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(jsonArgs, &args); err != nil {
		return errEnvelope(errCodeInvalidArgs, fmt.Sprintf("invalid JSON args: %v", err)), nil
	}
	taskID := args.TaskID
	if taskID == "" {
		id, err := t.latestActiveTaskID(ctx, sessionID)
		if err != nil {
			return errEnvelope(errCodeNoActiveTask, err.Error()), nil
		}
		taskID = id
	}
	if err := t.svc.CancelTask(ctx, taskID); err != nil {
		return errEnvelope(errCodeCancelFailed, err.Error()), nil
	}
	return envelope("ok", true, "task_id", taskID, "status", string(agentbridge.StatusCancelling)), nil
}

// latestActiveTaskID 找本会话最近的非终态任务（取消缺省目标）。
func (t *cancelTool) latestActiveTaskID(ctx context.Context, sessionID string) (string, error) {
	tasks, err := t.svc.ListSessionTasks(ctx, sessionID, 20)
	if err != nil {
		return "", err
	}
	for _, task := range tasks {
		if !task.Status.IsTerminal() {
			return task.ID, nil
		}
	}
	return "", errors.New("no active coding task in this session")
}

// --- interface guards ---

var (
	_ trpctool.ToolSet      = (*ToolSet)(nil)
	_ trpctool.Tool         = (*dispatchTool)(nil)
	_ trpctool.CallableTool = (*dispatchTool)(nil)
	_ trpctool.Tool         = (*checkTool)(nil)
	_ trpctool.CallableTool = (*checkTool)(nil)
	_ trpctool.Tool         = (*cancelTool)(nil)
	_ trpctool.CallableTool = (*cancelTool)(nil)
)
