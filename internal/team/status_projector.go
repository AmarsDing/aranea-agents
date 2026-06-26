package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// OrchestrationProjectorConfig configures a run-scoped status projector.
type OrchestrationProjectorConfig struct {
	RunID            string
	TeamID           string
	SessionID        string
	Registry         biz.OrchestrationRegistry
	Channel          string // "team" or "graph"; defaults to team
	GraphExecutionID string
	ActivityFlusher  *ActivityStepFlusher
	FailureOnError   string // await_review | halt (FP-03)

	// ActivityBus receives team_step and graph_stage ActivityEvents for
	// orchestration status projection. Outgoing orchestration_agent_status
	// events are also published to this bus.
	ActivityBus biz.ActivityEventBus
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

// StartOrchestrationStatusProjector subscribes to ActivityBus for team_step and
// graph_stage events, projects them onto orchestration node states, and publishes
// orchestration_agent_status events back to ActivityBus.
func StartOrchestrationStatusProjector(ctx context.Context, cfg OrchestrationProjectorConfig) context.CancelFunc {
	if cfg.ActivityBus == nil || strings.TrimSpace(cfg.SessionID) == "" {
		return func() {}
	}
	channel := strings.TrimSpace(cfg.Channel)
	if channel == "" {
		channel = "team"
	}
	store := biz.NewOrchestrationStatusStore(cfg.Registry)
	procCtx, cancel := context.WithCancel(ctx)

	ach, aunsub := cfg.ActivityBus.Subscribe(biz.ActivityEventSubscribeOptions{
		SessionID:  cfg.SessionID,
		BufferSize: 128,
	})
	safego.Go(procCtx, "orchestration.status.projector", func() {
		defer aunsub()
		for {
			select {
			case <-procCtx.Done():
				return
			case aev, ok := <-ach:
				if !ok {
					return
				}
				if aev.Activity.Kind == biz.ActivityKindNotice && aev.Activity.Stage == "orchestration_status" {
					continue
				}
				changed := store.ApplyActivityEvent(aev, cfg.Registry)
				for _, st := range changed {
					publishOrchestrationStatus(procCtx, cfg.ActivityBus, cfg, channel, st)
				}
			}
		}
	})

	return cancel
}

func publishOrchestrationStatus(ctx context.Context, bus biz.ActivityEventBus, cfg OrchestrationProjectorConfig, channel string, st *biz.AgentNodeState) {
	if st == nil || bus == nil {
		return
	}
	status := st.Status
	displayStatus := st.DisplayStatus
	if strings.EqualFold(strings.TrimSpace(cfg.FailureOnError), "await_review") && status == biz.AgentNodeStatusFailed {
		status = biz.AgentNodeStatusWaitingReview
		displayStatus = biz.DisplayStatusSuspended
	}
	meta := map[string]any{
		"run_id":         cfg.RunID,
		"team_id":        cfg.TeamID,
		"node_id":        st.NodeID,
		"agent_id":       st.AgentID,
		"agent_key":      st.AgentKey,
		"agent_name":     st.AgentName,
		"role":           st.Role,
		"status":         string(status),
		"display_status": string(displayStatus),
		"phase":          string(st.Phase),
		"retry_count":    st.RetryCount,
		"input_preview":  st.InputPreview,
		"output_preview": st.OutputPreview,
		"error_message":  st.ErrorMessage,
		"channel":        channel,
		"filter_key":     fmt.Sprintf("orchestration/%s/%s", cfg.RunID, st.NodeID),
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

	ev := biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:        uuid.NewString(),
			Kind:      biz.ActivityKindNotice,
			Status:    agentNodeStatusToActivityStatus(status),
			SessionID: cfg.SessionID,
			TeamID:    cfg.TeamID,
			AgentKey:  st.AgentKey,
			Timestamp: time.Now().UTC(),
			Stage:     "orchestration_status",
			Meta:      meta,
		},
		Domain: biz.ActivityDomainChat,
	}
	bus.Publish(ctx, ev)
}

// agentNodeStatusToActivityStatus maps an AgentNodeStatus to the closest
// ActivityStatus for the ActivityEvent published by the projector.
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
