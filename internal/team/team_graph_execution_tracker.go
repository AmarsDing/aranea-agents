package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// TeamGraphExecutionRegistry tracks team GraphAgent executions for task/resume (M53 Phase 7).
type TeamGraphExecutionRegistry interface {
	RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, cfg biz.GraphBuildConfig) error
	MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error
}

// TeamGraphExecutionTrackerConfig wires checkpoint envelopes to execution registry updates.
type TeamGraphExecutionTrackerConfig struct {
	SessionID        string
	GraphExecutionID string
	Registry         TeamGraphExecutionRegistry
}

// StartTeamGraphExecutionTracker marks waiting_human when graph checkpoint interrupts fire.
func StartTeamGraphExecutionTracker(ctx context.Context, bus event.Bus, cfg TeamGraphExecutionTrackerConfig) context.CancelFunc {
	if bus == nil || cfg.Registry == nil {
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
	safego.Go(procCtx, "team.graph.exec.tracker", func() {
		defer unsub()
		for {
			select {
			case <-procCtx.Done():
				return
			case env, ok := <-ch:
				if !ok {
					return
				}
				if env.Type != event.EnvelopeTypeCheckpoint {
					continue
				}
				if trackerMetaString(env.Metadata, "execution_id") != execID {
					continue
				}
				nodeID := trackerMetaString(env.Metadata, "node_id")
				if nodeID == "" {
					nodeID = trackerMetaString(env.Metadata, "interrupt_key")
				}
				lineageID := trackerMetaString(env.Metadata, "lineage_id")
				if markErr := cfg.Registry.MarkTeamGraphInterrupt(procCtx, execID, nodeID, lineageID); markErr != nil {
					loggateway.Global().Warn("MarkTeamGraphInterrupt failed",
					loggateway.StepID("team.graph.interrupt_mark_fail"),
					loggateway.Str("exec_id", execID), loggateway.Str("node_id", nodeID), loggateway.Str("error", markErr.Error()))
				}
			}
		}
	})
	return cancel
}

func trackerMetaString(meta map[string]any, key string) string {
	return bridgeMetaString(meta, key)
}
