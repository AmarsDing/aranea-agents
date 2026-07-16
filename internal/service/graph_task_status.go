package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// PublishGraphTaskStatus emits orchestration-facing task status for graph Kanban projection
// as a system.notice (NoticeType=graph_task_status).
func (p *GraphOrchestrationProjector) PublishGraphTaskStatus(ctx context.Context, sessionID, execID, graphID string, task *biz.GraphTask, extra map[string]any) {
	if p == nil || p.eventBus == nil || task == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	execID = strings.TrimSpace(execID)
	if sessionID == "" || execID == "" {
		return
	}
	graphID = strings.TrimSpace(graphID)
	meta := map[string]any{
		"execution_id":   execID,
		"graph_id":       graphID,
		"node_id":        task.NodeID,
		"task_id":        task.TaskID,
		"task_status":    string(task.Status),
		"assignee":       task.Assignee,
		"summary":        task.Summary,
		"webhook_topic":  "graph.task.status",
		"filter_key":     "graph/" + graphID + "/" + execID,
		"channel":        "graph",
		"author":         "graph-task",
		"activity_kind":  string(biz.ActivityKindGraphStage),
		"activity_status": string(biz.ActivityStatusRunning),
		"activity_event": string(biz.ActivityEventUpdated),
	}
	for k, v := range extra {
		meta[k] = v
	}
	ev := biz.NewSystemNoticeEvent(sessionID, "graph_task_status", "", meta)
	p.mu.Lock()
	seq := p.seq
	p.mu.Unlock()
	if seq != nil {
		seq.Publish(ctx, ev)
		return
	}
	p.eventBus.Publish(ctx, ev)
}
