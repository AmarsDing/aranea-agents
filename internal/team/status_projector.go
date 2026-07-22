package team

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
)

// OrchestrationProjectorConfig configures a run-scoped status projector.
type OrchestrationProjectorConfig struct {
	RunID            string
	TeamID           string
	SessionID        string
	SpiritSessionID  string
	Registry         biz.OrchestrationRegistry
	Channel          string // "team" or "graph"; defaults to team
	GraphExecutionID string
	ActivityFlusher  *ActivityStepFlusher
	FailureOnError   string // await_review | halt (FP-03)

	// EventBus receives system.notice (and other) events for orchestration
	// status projection. Outgoing orchestration_status notices are published
	// back to this bus (or Sequencer when set).
	EventBus biz.EventBus
	// Sequencer routes orchestration_status Notice events through the v2
	// Sequencer (FIFO + retry); falls back to EventBus when nil.
	Sequencer rt.EventPublisher
}

// BuildOrchestrationRegistry maps team members to graph node IDs (member-{sort_order}).
func BuildOrchestrationRegistry(def Definition, agentKey func(agentID string) string, agentName func(agentID string) string) biz.OrchestrationRegistry {
	members := EnabledMembers(def)
	entries := make([]biz.OrchestrationNodeRegistryEntry, 0, len(members))
	for i, m := range members {
		agentID := strings.TrimSpace(m.AgentID)
		if agentID == "" {
			continue
		}
		sortOrder := m.SortOrder
		if sortOrder <= 0 {
			sortOrder = i + 1
		}
		key := agentKey(agentID)
		name := agentName(agentID)
		if name == "" {
			name = strings.TrimSpace(m.Name)
		}
		if name == "" {
			name = key
		}
		entries = append(entries, biz.OrchestrationNodeRegistryEntry{
			NodeID:    fmt.Sprintf("member-%d", sortOrder),
			AgentID:   agentID,
			AgentKey:  key,
			AgentName: name,
			Role:      strings.TrimSpace(m.Role),
		})
	}
	return biz.NewOrchestrationRegistry(entries)
}

// StartOrchestrationStatusProjector subscribes via teamRunPipeline (BL-09),
// projects ActivityEvents onto orchestration node states, and publishes
// orchestration_status notices back to the bus.
func StartOrchestrationStatusProjector(ctx context.Context, cfg OrchestrationProjectorConfig) context.CancelFunc {
	if cfg.EventBus == nil || strings.TrimSpace(cfg.SessionID) == "" {
		return func() {}
	}
	channel := strings.TrimSpace(cfg.Channel)
	if channel == "" {
		channel = "team"
	}
	handler := &orchestrationStatusHandler{
		cfg:     cfg,
		store:   biz.NewOrchestrationStatusStore(cfg.Registry),
		channel: channel,
	}
	return newTeamRunPipeline(handler).Start(ctx, cfg.EventBus, cfg.SpiritSessionID, cfg.SessionID)
}

func publishOrchestrationStatus(ctx context.Context, cfg OrchestrationProjectorConfig, channel string, st *biz.AgentNodeState) {
	if st == nil || (cfg.Sequencer == nil && cfg.EventBus == nil) {
		return
	}
	status := st.Status
	displayStatus := st.DisplayStatus
	if strings.EqualFold(strings.TrimSpace(cfg.FailureOnError), "await_review") && status == biz.AgentNodeStatusFailed {
		status = biz.AgentNodeStatusWaitingReview
		displayStatus = biz.DisplayStatusSuspended
	}
	meta := map[string]any{
		"run_id":          cfg.RunID,
		"team_id":         cfg.TeamID,
		"node_id":         st.NodeID,
		"agent_id":        st.AgentID,
		"agent_key":       st.AgentKey,
		"agent_name":      st.AgentName,
		"role":            st.Role,
		"status":          string(status),
		"display_status":  string(displayStatus),
		"phase":           string(st.Phase),
		"retry_count":     st.RetryCount,
		"input_preview":   st.InputPreview,
		"output_preview":  st.OutputPreview,
		"error_message":   st.ErrorMessage,
		"channel":         channel,
		"filter_key":      fmt.Sprintf("orchestration/%s/%s", cfg.RunID, st.NodeID),
		"activity_kind":   string(biz.ActivityKindNotice),
		"activity_status": string(agentNodeStatusToActivityStatus(status)),
		"activity_event":  string(biz.ActivityEventUpdated),
	}
	if st.CurrentActivity != nil {
		meta["current_activity"] = st.CurrentActivity
	}
	if len(st.ActivityHistory) > 0 {
		meta["activity_history"] = st.ActivityHistory
	}
	if cfg.ActivityFlusher != nil && len(st.ActivityHistory) > 0 {
		last := st.ActivityHistory[len(st.ActivityHistory)-1]
		cfg.ActivityFlusher.Enqueue(st.NodeID, last)
	}

	sessionID := cfg.SpiritSessionID
	if sessionID == "" {
		sessionID = cfg.SessionID
	}
	ev := biz.NewSystemNoticeEvent(sessionID, "orchestration_status", "", meta)
	if cfg.Sequencer != nil {
		cfg.Sequencer.Publish(ctx, ev)
	} else {
		cfg.EventBus.Publish(ctx, ev)
	}
}

// agentNodeStatusToActivityStatus maps an AgentNodeStatus to the closest
// ActivityStatus for legacy consumers reconstructing ActivityEvent from notices.
func agentNodeStatusToActivityStatus(s biz.AgentNodeStatus) biz.ActivityStatus {
	switch s {
	case biz.AgentNodeStatusIdle, biz.AgentNodeStatusQueued, biz.AgentNodeStatusScheduled:
		return biz.ActivityStatusPending
	case biz.AgentNodeStatusRunning, biz.AgentNodeStatusThinking, biz.AgentNodeStatusTransferring, biz.AgentNodeStatusRetrying:
		return biz.ActivityStatusRunning
	case biz.AgentNodeStatusToolRunning:
		return biz.ActivityStatusToolRunning
	case biz.AgentNodeStatusWaitingInput, biz.AgentNodeStatusWaitingReview, biz.AgentNodeStatusWaitingAssign, biz.AgentNodeStatusBlocked:
		return biz.ActivityStatusToolBlocked
	case biz.AgentNodeStatusSuccess:
		return biz.ActivityStatusCompleted
	case biz.AgentNodeStatusFailed, biz.AgentNodeStatusTimedOut:
		return biz.ActivityStatusFailed
	case biz.AgentNodeStatusSkipped, biz.AgentNodeStatusCancelled:
		return biz.ActivityStatusCancelled
	default:
		return biz.ActivityStatusPending
	}
}
