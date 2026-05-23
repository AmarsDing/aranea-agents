package team

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

// OrchestrationProjectorConfig configures a run-scoped status projector.
type OrchestrationProjectorConfig struct {
	RunID     string
	TeamID    string
	SessionID string
	Registry  biz.OrchestrationRegistry
	Channel   string // "team" or "graph"; defaults to team
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
func StartOrchestrationStatusProjector(ctx context.Context, bus event.Bus, cfg OrchestrationProjectorConfig) context.CancelFunc {
	if bus == nil || strings.TrimSpace(cfg.SessionID) == "" {
		return func() {}
	}
	channel := strings.TrimSpace(cfg.Channel)
	if channel == "" {
		channel = "team"
	}
	store := biz.NewOrchestrationStatusStore(cfg.Registry)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{
		SessionID:  cfg.SessionID,
		BufferSize: 128,
		DropPolicy: event.DropNewest,
	})
	procCtx, cancel := context.WithCancel(ctx)
	safego.Go(procCtx, "orchestration.status.projector", func() {
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
					publishOrchestrationStatus(procCtx, bus, cfg, channel, st)
				}
			}
		}
	})
	return cancel
}

func publishOrchestrationStatus(ctx context.Context, bus event.Bus, cfg OrchestrationProjectorConfig, channel string, st *biz.AgentNodeState) {
	if bus == nil || st == nil {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeOrchestrationAgentStatus, "orchestration-projector", cfg.SessionID)
	env.TeamID = cfg.TeamID
	env.Channel = channel
	env.FilterKey = fmt.Sprintf("orchestration/%s/%s", cfg.RunID, st.NodeID)
	meta := map[string]any{
		"run_id":         cfg.RunID,
		"team_id":        cfg.TeamID,
		"node_id":        st.NodeID,
		"agent_id":       st.AgentID,
		"agent_key":      st.AgentKey,
		"agent_name":     st.AgentName,
		"role":           st.Role,
		"status":         string(st.Status),
		"display_status": string(st.DisplayStatus),
		"phase":          string(st.Phase),
		"retry_count":    st.RetryCount,
		"input_preview":  st.InputPreview,
		"output_preview": st.OutputPreview,
		"error_message":  st.ErrorMessage,
	}
	if st.CurrentActivity != nil {
		meta["current_activity"] = st.CurrentActivity
	}
	if len(st.ActivityHistory) > 0 {
		meta["activity_history"] = st.ActivityHistory
	}
	env.Metadata = meta
	bus.Publish(ctx, env)
}
