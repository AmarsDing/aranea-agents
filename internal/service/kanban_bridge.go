package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	kanbanpkg "aranea-agents/internal/tools/kanban"
)

// KanbanToolBridge implements kanban.Bridge using TaskUsecase.
type KanbanToolBridge struct {
	tasks *biz.TaskUsecase
}

func NewKanbanToolBridge(tasks *biz.TaskUsecase) *KanbanToolBridge {
	return &KanbanToolBridge{tasks: tasks}
}

var _ kanbanpkg.Bridge = (*KanbanToolBridge)(nil)

func (b *KanbanToolBridge) Show(ctx context.Context, taskID string) (map[string]any, error) {
	task, err := b.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	comments, _ := b.tasks.ListTaskComments(ctx, taskID)
	events, _ := b.tasks.ListTaskEvents(ctx, task.ExecutionID, taskID, "", 10)
	return map[string]any{
		"task":     taskToMap(task),
		"comments": comments,
		"events":   events,
	}, nil
}

func (b *KanbanToolBridge) List(ctx context.Context, executionID, status string, limit int) ([]map[string]any, error) {
	items, _, err := b.tasks.ListTasks(ctx, executionID, biz.TaskStatus(status), limit, "")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(items))
	for i, t := range items {
		out[i] = taskToMap(t)
	}
	return out, nil
}

func (b *KanbanToolBridge) Complete(ctx context.Context, taskID, summary, output, metadata string) (map[string]any, error) {
	task, err := b.tasks.SubmitTaskResult(ctx, taskID, output, summary, metadata)
	if err != nil {
		return nil, err
	}
	return taskToMap(task), nil
}

func (b *KanbanToolBridge) Block(ctx context.Context, taskID, reason, metadata string) (map[string]any, error) {
	task, err := b.tasks.ReportBlocked(ctx, taskID, reason, metadata)
	if err != nil {
		return nil, err
	}
	return taskToMap(task), nil
}

func (b *KanbanToolBridge) Unblock(ctx context.Context, taskID, comment string) (map[string]any, error) {
	task, err := b.tasks.UnblockTask(ctx, taskID, comment)
	if err != nil {
		return nil, err
	}
	return taskToMap(task), nil
}

func (b *KanbanToolBridge) Heartbeat(ctx context.Context, taskID, agentKey, metadata string) (map[string]any, error) {
	ack, ext, err := b.tasks.Heartbeat(ctx, taskID, agentKey, metadata)
	if err != nil {
		return nil, err
	}
	return map[string]any{"acknowledged": ack, "lease_extension_seconds": ext}, nil
}

func (b *KanbanToolBridge) Comment(ctx context.Context, taskID, author, body, commentType string) (map[string]any, error) {
	if commentType == "" {
		commentType = "note"
	}
	c, err := b.tasks.AddTaskComment(ctx, taskID, author, body, commentType)
	if err != nil {
		return nil, err
	}
	return map[string]any{"comment_id": c.CommentID}, nil
}

func (b *KanbanToolBridge) Create(ctx context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error) {
	if nodeID == "" {
		nodeID = "kanban-" + strings.ReplaceAll(title, " ", "-")
	}
	task, err := b.tasks.CreateTaskWithParents(ctx, biz.CreateTaskParams{
		ExecutionID:    executionID,
		NodeID:         nodeID,
		AssignmentMode: "static",
		Input:          input,
		Context:        "{}",
		ParentIDs:      parentIDs,
	})
	if err != nil {
		return nil, err
	}
	if assignee != "" {
		claimed, err := b.tasks.ClaimTask(ctx, task.TaskID, assignee)
		if err == nil {
			task = claimed
		}
	}
	return taskToMap(task), nil
}

func (b *KanbanToolBridge) Link(ctx context.Context, parentTaskID, childTaskID string) error {
	return b.tasks.LinkTasks(ctx, parentTaskID, childTaskID)
}

func taskToMap(task *biz.GraphTask) map[string]any {
	if task == nil {
		return nil
	}
	return map[string]any{
		"task_id":      task.TaskID,
		"node_id":      task.NodeID,
		"execution_id": task.ExecutionID,
		"assignee":     task.Assignee,
		"status":       string(task.Status),
		"input":        task.Input,
		"output":       task.Output,
		"summary":      task.Summary,
	}
}
