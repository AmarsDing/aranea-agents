package kanban

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func NewToolset(b Bridge) []trpctool.CallableTool {
	if b == nil {
		return nil
	}
	return []trpctool.CallableTool{
		&tool{name: "kanban_show", desc: "Read the current graph task (title, input, status, comments summary). Defaults to ARANEA_TASK_ID env.", bridge: b, fn: showFn, schema: showSchema},
		&tool{name: "kanban_list", desc: "List tasks for an execution with optional status filter.", bridge: b, fn: listFn, schema: listSchema},
		&tool{name: "kanban_complete", desc: "Mark task complete with summary and optional output/metadata JSON.", bridge: b, fn: completeFn, schema: completeSchema},
		&tool{name: "kanban_block", desc: "Block task and escalate for human input.", bridge: b, fn: blockFn, schema: blockSchema},
		&tool{name: "kanban_unblock", desc: "Move a blocked task back to pending.", bridge: b, fn: unblockFn, schema: unblockSchema},
		&tool{name: "kanban_heartbeat", desc: "Signal liveness for a claimed task.", bridge: b, fn: heartbeatFn, schema: heartbeatSchema},
		&tool{name: "kanban_comment", desc: "Append a durable comment to the task thread.", bridge: b, fn: commentFn, schema: commentSchema},
		&tool{name: "kanban_create", desc: "Create a child task on the current execution board.", bridge: b, fn: createFn, schema: createSchema},
		&tool{name: "kanban_link", desc: "Add parent→child dependency between tasks.", bridge: b, fn: linkFn, schema: linkSchema},
	}
}

type tool struct {
	name   string
	desc   string
	bridge Bridge
	fn     func(context.Context, Bridge, []byte) (any, error)
	schema *trpctool.Schema
}

func (t *tool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: t.name, Description: t.desc, InputSchema: t.schema}
}

func (t *tool) Call(ctx context.Context, args []byte) (any, error) {
	if t.bridge == nil {
		return nil, apierror.Internal(apierror.DomainKanban, t.name+": kanban bridge not configured")
	}
	return t.fn(ctx, t.bridge, args)
}

func (t *tool) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	return nil, apierror.BadRequest(apierror.DomainKanban, t.name+": kanban tools are not streamable")
}

const strType = "string"
const intType = "integer"
const arrType = "array"

func showFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_show: invalid args: "+err.Error())
	}
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_show: task_id required")
	}
	return b.Show(ctx, taskID)
}

func listFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		ExecutionID string `json:"execution_id"`
		Status      string `json:"status"`
		Limit       int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_list: invalid args: "+err.Error())
	}
	execID := strings.TrimSpace(in.ExecutionID)
	if execID == "" {
		execID = ExecutionIDFromEnv()
	}
	if execID == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_list: execution_id required")
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}
	items, err := b.List(ctx, execID, in.Status, in.Limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items}, nil
}

func completeFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID   string `json:"task_id"`
		AgentKey string `json:"agent_key"`
		Summary  string `json:"summary"`
		Result   string `json:"result"`
		Output   string `json:"output"`
		Metadata string `json:"metadata"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_complete: invalid args: "+err.Error())
	}
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	// agent_key 可选：缺省回退环境变量（与 kanban_heartbeat 同模式）。
	// 提供后 biz 层启用 assignee CAS 守卫，防止误提交他人任务。
	agentKey := strings.TrimSpace(in.AgentKey)
	if agentKey == "" {
		agentKey = lookupEnv("ARANEA_AGENT_KEY")
	}
	output := strings.TrimSpace(in.Output)
	if output == "" {
		output = strings.TrimSpace(in.Result)
	}
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		summary = output
	}
	if taskID == "" || summary == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_complete: task_id and summary/result required")
	}
	return b.Complete(ctx, taskID, agentKey, summary, output, in.Metadata)
}

func blockFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID   string `json:"task_id"`
		Reason   string `json:"reason"`
		Metadata string `json:"metadata"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_block: invalid args: "+err.Error())
	}
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	if taskID == "" || strings.TrimSpace(in.Reason) == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_block: task_id and reason required")
	}
	return b.Block(ctx, taskID, in.Reason, in.Metadata)
}

func unblockFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID  string `json:"task_id"`
		Comment string `json:"comment"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_unblock: invalid args: "+err.Error())
	}
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_unblock: task_id required")
	}
	return b.Unblock(ctx, taskID, in.Comment)
}

func heartbeatFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID   string `json:"task_id"`
		AgentKey string `json:"agent_key"`
		Metadata string `json:"metadata"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_heartbeat: invalid args: "+err.Error())
	}
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	agentKey := strings.TrimSpace(in.AgentKey)
	if agentKey == "" {
		agentKey = lookupEnv("ARANEA_AGENT_KEY")
	}
	if taskID == "" || agentKey == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_heartbeat: task_id and agent_key required")
	}
	return b.Heartbeat(ctx, taskID, agentKey, in.Metadata)
}

func commentFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID  string `json:"task_id"`
		Author  string `json:"author"`
		Body    string `json:"body"`
		Content string `json:"content"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_comment: invalid args: "+err.Error())
	}
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		body = strings.TrimSpace(in.Content)
	}
	author := strings.TrimSpace(in.Author)
	if author == "" {
		author = lookupEnv("ARANEA_AGENT_KEY")
	}
	if taskID == "" || body == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_comment: task_id and body required")
	}
	return b.Comment(ctx, taskID, author, body, in.Type)
}

func createFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		ExecutionID string   `json:"execution_id"`
		NodeID      string   `json:"node_id"`
		Title       string   `json:"title"`
		Assignee    string   `json:"assignee"`
		Input       string   `json:"input"`
		Parents     []string `json:"parents"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_create: invalid args: "+err.Error())
	}
	execID := strings.TrimSpace(in.ExecutionID)
	if execID == "" {
		execID = ExecutionIDFromEnv()
	}
	if execID == "" || strings.TrimSpace(in.Title) == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_create: execution_id and title required")
	}
	input := strings.TrimSpace(in.Input)
	if input == "" {
		input = in.Title
	}
	return b.Create(ctx, execID, in.NodeID, in.Title, in.Assignee, input, in.Parents)
}

func linkFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		ParentID string `json:"parent_id"`
		ChildID  string `json:"child_id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_link: invalid args: "+err.Error())
	}
	if strings.TrimSpace(in.ParentID) == "" || strings.TrimSpace(in.ChildID) == "" {
		return nil, apierror.BadRequest(apierror.DomainKanban, "kanban_link: parent_id and child_id required")
	}
	if err := b.Link(ctx, in.ParentID, in.ChildID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func Enabled() bool {
	return strings.TrimSpace(os.Getenv("ARANEA_TASK_ID")) != "" ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("ARANEA_KANBAN_TOOLS")), "1")
}

var (
	showSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"task_id": {Type: strType, Description: "Task ID (defaults to ARANEA_TASK_ID env)"},
		},
	}

	listSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"execution_id": {Type: strType, Description: "Execution ID (defaults to ARANEA_EXECUTION_ID env)"},
			"status":       {Type: strType, Description: "Optional status filter (pending/running/completed/blocked)"},
			"limit":        {Type: intType, Description: "Max results (default 20)"},
		},
	}

	completeSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"task_id":   {Type: strType, Description: "Task ID (defaults to ARANEA_TASK_ID env)"},
			"agent_key": {Type: strType, Description: "Submitter agent key (defaults to ARANEA_AGENT_KEY env); validated against task assignee"},
			"summary":   {Type: strType, Description: "Completion summary"},
			"result":    {Type: strType, Description: "Alias for output"},
			"output":    {Type: strType, Description: "Structured output data"},
			"metadata":  {Type: strType, Description: "Optional metadata JSON"},
		},
	}

	blockSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"task_id":  {Type: strType, Description: "Task ID (defaults to ARANEA_TASK_ID env)"},
			"reason":   {Type: strType, Description: "Reason for blocking"},
			"metadata": {Type: strType, Description: "Optional metadata JSON"},
		},
		Required: []string{"reason"},
	}

	unblockSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"task_id": {Type: strType, Description: "Task ID (defaults to ARANEA_TASK_ID env)"},
			"comment": {Type: strType, Description: "Optional unblock comment"},
		},
	}

	heartbeatSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"task_id":   {Type: strType, Description: "Task ID (defaults to ARANEA_TASK_ID env)"},
			"agent_key": {Type: strType, Description: "Agent key (defaults to ARANEA_AGENT_KEY env)"},
			"metadata":  {Type: strType, Description: "Optional metadata JSON"},
		},
	}

	commentSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"task_id": {Type: strType, Description: "Task ID (defaults to ARANEA_TASK_ID env)"},
			"author":  {Type: strType, Description: "Comment author (defaults to ARANEA_AGENT_KEY env)"},
			"body":    {Type: strType, Description: "Comment body text"},
			"content": {Type: strType, Description: "Alias for body"},
			"type":    {Type: strType, Description: "Comment type"},
		},
		Required: []string{"body"},
	}

	createSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"execution_id": {Type: strType, Description: "Execution ID (defaults to ARANEA_EXECUTION_ID env)"},
			"node_id":      {Type: strType, Description: "Graph node ID"},
			"title":        {Type: strType, Description: "Task title"},
			"assignee":     {Type: strType, Description: "Assignee agent key"},
			"input":        {Type: strType, Description: "Task input (defaults to title)"},
			"parents":      {Type: arrType, Items: &trpctool.Schema{Type: strType}, Description: "Parent task IDs"},
		},
		Required: []string{"title"},
	}

	linkSchema = &trpctool.Schema{
		Type: "object",
		Properties: map[string]*trpctool.Schema{
			"parent_id": {Type: strType, Description: "Parent task ID"},
			"child_id":  {Type: strType, Description: "Child task ID"},
		},
		Required: []string{"parent_id", "child_id"},
	}
)
