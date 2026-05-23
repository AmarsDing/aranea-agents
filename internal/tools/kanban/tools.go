package kanban

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func NewToolset() []trpctool.CallableTool {
	return []trpctool.CallableTool{
		&tool{name: "kanban_show", desc: "Read the current graph task (title, input, status, comments summary). Defaults to ARANEA_TASK_ID env.", fn: showFn},
		&tool{name: "kanban_list", desc: "List tasks for an execution with optional status filter.", fn: listFn},
		&tool{name: "kanban_complete", desc: "Mark task complete with summary and optional output/metadata JSON.", fn: completeFn},
		&tool{name: "kanban_block", desc: "Block task and escalate for human input.", fn: blockFn},
		&tool{name: "kanban_unblock", desc: "Move a blocked task back to pending.", fn: unblockFn},
		&tool{name: "kanban_heartbeat", desc: "Signal liveness for a claimed task.", fn: heartbeatFn},
		&tool{name: "kanban_comment", desc: "Append a durable comment to the task thread.", fn: commentFn},
		&tool{name: "kanban_create", desc: "Create a child task on the current execution board.", fn: createFn},
		&tool{name: "kanban_link", desc: "Add parent→child dependency between tasks.", fn: linkFn},
	}
}

type tool struct {
	name string
	desc string
	fn   func(context.Context, Bridge, []byte) (any, error)
}

func (t *tool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: t.name, Description: t.desc}
}

func (t *tool) Call(ctx context.Context, args []byte) (any, error) {
	b := BridgeFromContext(ctx)
	if b == nil {
		return nil, fmt.Errorf("%s: kanban bridge not configured", t.name)
	}
	return t.fn(ctx, b, args)
}

func showFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID string `json:"task_id"`
	}
	_ = json.Unmarshal(args, &in)
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	if taskID == "" {
		return nil, fmt.Errorf("kanban_show: task_id required")
	}
	return b.Show(ctx, taskID)
}

func listFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		ExecutionID string `json:"execution_id"`
		Status      string `json:"status"`
		Limit       int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &in)
	execID := strings.TrimSpace(in.ExecutionID)
	if execID == "" {
		execID = ExecutionIDFromEnv()
	}
	if execID == "" {
		return nil, fmt.Errorf("kanban_list: execution_id required")
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
		Summary  string `json:"summary"`
		Result   string `json:"result"`
		Output   string `json:"output"`
		Metadata string `json:"metadata"`
	}
	_ = json.Unmarshal(args, &in)
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
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
		return nil, fmt.Errorf("kanban_complete: task_id and summary/result required")
	}
	return b.Complete(ctx, taskID, summary, output, in.Metadata)
}

func blockFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID   string `json:"task_id"`
		Reason   string `json:"reason"`
		Metadata string `json:"metadata"`
	}
	_ = json.Unmarshal(args, &in)
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	if taskID == "" || strings.TrimSpace(in.Reason) == "" {
		return nil, fmt.Errorf("kanban_block: task_id and reason required")
	}
	return b.Block(ctx, taskID, in.Reason, in.Metadata)
}

func unblockFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID  string `json:"task_id"`
		Comment string `json:"comment"`
	}
	_ = json.Unmarshal(args, &in)
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	if taskID == "" {
		return nil, fmt.Errorf("kanban_unblock: task_id required")
	}
	return b.Unblock(ctx, taskID, in.Comment)
}

func heartbeatFn(ctx context.Context, b Bridge, args []byte) (any, error) {
	var in struct {
		TaskID   string `json:"task_id"`
		AgentKey string `json:"agent_key"`
		Metadata string `json:"metadata"`
	}
	_ = json.Unmarshal(args, &in)
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		taskID = TaskIDFromEnv()
	}
	agentKey := strings.TrimSpace(in.AgentKey)
	if agentKey == "" {
		agentKey = lookupEnv("ARANEA_AGENT_KEY")
	}
	if taskID == "" || agentKey == "" {
		return nil, fmt.Errorf("kanban_heartbeat: task_id and agent_key required")
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
	_ = json.Unmarshal(args, &in)
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
		return nil, fmt.Errorf("kanban_comment: task_id and body required")
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
	_ = json.Unmarshal(args, &in)
	execID := strings.TrimSpace(in.ExecutionID)
	if execID == "" {
		execID = ExecutionIDFromEnv()
	}
	if execID == "" || strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("kanban_create: execution_id and title required")
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
	_ = json.Unmarshal(args, &in)
	if strings.TrimSpace(in.ParentID) == "" || strings.TrimSpace(in.ChildID) == "" {
		return nil, fmt.Errorf("kanban_link: parent_id and child_id required")
	}
	if err := b.Link(ctx, in.ParentID, in.ChildID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

// Enabled reports whether kanban tools should be mounted (task worker or explicit env).
func Enabled() bool {
	return strings.TrimSpace(os.Getenv("ARANEA_TASK_ID")) != "" ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("ARANEA_KANBAN_TOOLS")), "1")
}
