package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
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

	// ActivityBus, when non-nil, enables dual-bus operation:
	//   - incoming team_step events are consumed from ActivityBus (replacing legacy envelopes)
	//   - outgoing orchestration_agent_status events are published to ActivityBus
	// When nil, the projector falls back to the legacy event.Bus for both directions
	// (used by graph-domain callers that have not yet migrated).
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

// StartOrchestrationStatusProjector subscribes to session envelopes and publishes orchestration_agent_status.
//
// When cfg.ActivityBus is non-nil, team_step events are consumed from the
// ActivityEventBus (translated to synthetic envelopes for the status store)
// and orchestration_agent_status events are published to ActivityBus.
// The legacy event.Bus is still subscribed for non-team events (graph node
// lifecycle, run_status, etc.) that have not been migrated.
func StartOrchestrationStatusProjector(ctx context.Context, bus event.Bus, cfg OrchestrationProjectorConfig) context.CancelFunc {
	if (bus == nil && cfg.ActivityBus == nil) || strings.TrimSpace(cfg.SessionID) == "" {
		return func() {}
	}
	channel := strings.TrimSpace(cfg.Channel)
	if channel == "" {
		channel = "team"
	}
	store := biz.NewOrchestrationStatusStore(cfg.Registry)
	procCtx, cancel := context.WithCancel(ctx)

	// Legacy bus subscription: receives graph-domain events and any
	// not-yet-migrated envelopes. Skipped when bus is nil.
	if bus != nil {
		ch, unsub := bus.Subscribe(event.SubscribeOptions{
			SessionID:  cfg.SessionID,
			BufferSize: 128,
			DropPolicy: event.DropNewest,
		})
		safego.Go(procCtx, "orchestration.status.projector.legacy", func() {
			defer unsub()
			for {
				select {
				case <-procCtx.Done():
					return
				case env, ok := <-ch:
					if !ok {
						return
					}
					if env.Type == event.EnvelopeTypeOrchestrationAgentStatus {
						continue
					}
					changed := store.ApplyEnvelope(env, cfg.Registry)
					for _, st := range changed {
						publishOrchestrationStatus(procCtx, cfg.ActivityBus, cfg, channel, st)
					}
				}
			}
		})
	}

	// ActivityBus subscription: receives migrated team_step ActivityEvents,
	// translates them to synthetic envelopes, and feeds them to the status store.
	if cfg.ActivityBus != nil {
		ach, aunsub := cfg.ActivityBus.Subscribe(biz.ActivityEventSubscribeOptions{
			SessionID:  cfg.SessionID,
			BufferSize: 128,
		})
		safego.Go(procCtx, "orchestration.status.projector.activity", func() {
			defer aunsub()
			for {
				select {
				case <-procCtx.Done():
					return
				case aev, ok := <-ach:
					if !ok {
						return
					}
					env := activityEventToEnvelope(aev)
					if env.Type == "" {
						continue
					}
					changed := store.ApplyEnvelope(env, cfg.Registry)
					for _, st := range changed {
						publishOrchestrationStatus(procCtx, cfg.ActivityBus, cfg, channel, st)
					}
				}
			}
		})
	}

	return cancel
}

// activityEventToEnvelope translates a team_step ActivityEvent into a synthetic
// contract.Envelope so the existing OrchestrationStatusStore.ApplyEnvelope
// logic can process it without modification. Returns a zero-type Envelope
// (Type=="") when the event does not map to a team_step lifecycle transition.
func activityEventToEnvelope(aev biz.ActivityEvent) event.Envelope {
	if aev.Activity.Kind != biz.ActivityKindTeamStage {
		return event.Envelope{}
	}
	var typ event.EnvelopeType
	switch {
	case aev.Event == biz.ActivityEventCreated && aev.Activity.Stage == "executing":
		typ = event.EnvelopeTypeTeamStepStarted
	case aev.Event == biz.ActivityEventCompleted && aev.Activity.Stage == "completed":
		typ = event.EnvelopeTypeTeamStepFinished
	default:
		return event.Envelope{}
	}
	author := strings.TrimSpace(aev.Activity.AgentKey)
	env := event.NewEnvelope(typ, author, aev.Activity.SessionID)
	env.TeamID = aev.Activity.TeamID
	meta := aev.Activity.Meta
	if meta == nil {
		meta = map[string]any{}
	}
	if author != "" {
		if _, ok := meta["agent_key"]; !ok {
			meta["agent_key"] = author
		}
	}
	env.Metadata = meta
	return env
}

func publishOrchestrationStatus(ctx context.Context, bus biz.ActivityEventBus, cfg OrchestrationProjectorConfig, channel string, st *biz.AgentNodeState) {
	if st == nil {
		return
	}
	if bus == nil {
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

	// Publish orchestration_agent_status as ActivityEvent (Domain=chat).
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
