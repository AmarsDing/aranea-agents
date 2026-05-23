package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

// PublishGraphTaskStatus emits orchestration-facing task status for graph Kanban projection.
func (p *GraphOrchestrationProjector) PublishGraphTaskStatus(ctx context.Context, sessionID, execID, graphID string, task *biz.GraphTask, extra map[string]any) {
	if p == nil || p.bus == nil || task == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	execID = strings.TrimSpace(execID)
	if sessionID == "" || execID == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeGraphTaskStatus, "graph-task", sessionID)
	env.Channel = "graph"
	env.FilterKey = "graph/" + strings.TrimSpace(graphID) + "/" + execID
	env.Metadata = map[string]any{
		"execution_id": execID,
		"graph_id":     graphID,
		"node_id":      task.NodeID,
		"task_id":      task.TaskID,
		"task_status":  string(task.Status),
		"assignee":     task.Assignee,
		"summary":      task.Summary,
		"webhook_topic": "graph.task.status",
	}
	for k, v := range extra {
		env.Metadata[k] = v
	}
	p.bus.Publish(ctx, env)
}
