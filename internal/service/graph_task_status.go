package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// PublishGraphTaskStatus emits orchestration-facing task status for graph Kanban projection.
//
// Phase 3b-D: migrated from v1 ActivityEventBus to v2 EventBus via
// ActivityBridgeEvent. The v1 ActivityEvent payload is preserved verbatim
// (Meta carries graph task status fields) so the frontend Kanban UI and
// webhook consumers continue to work without changes.
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
		"execution_id":  execID,
		"graph_id":      graphID,
		"node_id":       task.NodeID,
		"task_id":       task.TaskID,
		"task_status":   string(task.Status),
		"assignee":      task.Assignee,
		"summary":       task.Summary,
		"webhook_topic": "graph.task.status",
		"filter_key":    "graph/" + graphID + "/" + execID,
		"channel":       "graph",
		"author":        "graph-task",
	}
	for k, v := range extra {
		meta[k] = v
	}
	ev := biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:        uuid.NewString(),
			Kind:      biz.ActivityKindGraphStage,
			Status:    biz.ActivityStatusRunning,
			SessionID: sessionID,
			Timestamp: time.Now().UTC(),
			Stage:     "task_status",
			Meta:      meta,
		},
		Domain: biz.ActivityDomainChat,
	}
	// Phase 3b-D: bridge to v2 EventBus. graph_stage has no typed v2 EventKind;
	// ActivityBridgeEvent preserves the v1 payload for the frontend Kanban UI.
	p.eventBus.Publish(ctx, biz.NewActivityBridgeEvent(ev))
}
