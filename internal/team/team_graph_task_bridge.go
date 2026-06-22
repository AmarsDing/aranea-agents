package team

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// TeamGraphTaskCreator creates Kanban task rows for team Graph runtime nodes (M53 TG-RT-TASK).
// Stability:evolving
type TeamGraphTaskCreator interface {
	CreateGraphTask(ctx context.Context, graphExecutionID, sessionID string, node biz.NodeDef) error
}

// TeamGraphTaskBridgeConfig wires graph_node_start envelopes to task creation for a team run.
type TeamGraphTaskBridgeConfig struct {
	SessionID        string
	GraphExecutionID string
	Nodes            map[string]biz.NodeDef
	Creator          TeamGraphTaskCreator
}

// TaskNodesFromBuildConfig indexes nodes that should spawn Kanban tasks at runtime.
func TaskNodesFromBuildConfig(cfg biz.GraphBuildConfig) map[string]biz.NodeDef {
	out := map[string]biz.NodeDef{}
	for _, n := range cfg.Nodes {
		node := n
		if biz.ShouldCreateTeamGraphTaskNode(&node) {
			out[node.ID] = node
		}
	}
	return out
}

// StartTeamGraphTaskBridge subscribes to session graph_node_start and creates tasks for configured nodes.
func StartTeamGraphTaskBridge(ctx context.Context, bus event.Bus, cfg TeamGraphTaskBridgeConfig, lg loggateway.Logger) context.CancelFunc {
	if bus == nil || cfg.Creator == nil || len(cfg.Nodes) == 0 {
		return func() {}
	}
	execID := strings.TrimSpace(cfg.GraphExecutionID)
	sessionID := strings.TrimSpace(cfg.SessionID)
	if execID == "" || sessionID == "" {
		return func() {}
	}
	ch, unsub := bus.Subscribe(event.SubscribeOptions{
		SessionID:  sessionID,
		BufferSize: 64,
		DropPolicy: event.DropNewest,
	})
	procCtx, cancel := context.WithCancel(ctx)
	safego.Go(procCtx, "team.graph.task.bridge", func() {
		defer unsub()
		created := map[string]struct{}{}
		for {
			select {
			case <-procCtx.Done():
				return
			case env, ok := <-ch:
				if !ok {
					return
				}
				if env.Type != event.EnvelopeTypeGraphNodeStart {
					continue
				}
				if bridgeMetaString(env.Metadata, "execution_id") != execID {
					continue
				}
				nodeID := bridgeMetaString(env.Metadata, "node_id")
				if nodeID == "" {
					continue
				}
				if _, done := created[nodeID]; done {
					continue
				}
				node, ok := cfg.Nodes[nodeID]
				if !ok {
					continue
				}
				if err := cfg.Creator.CreateGraphTask(procCtx, execID, sessionID, node); err != nil {
					lg.Warn("创建 Kanban 任务失败",
						loggateway.StepID("team.graph.task.bridge"),
						loggateway.Str("execution_id", execID),
						loggateway.Str("node_id", nodeID),
						loggateway.Err(err))
					continue
				}
				created[nodeID] = struct{}{}
			}
		}
	})
	return cancel
}

func bridgeMetaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}
