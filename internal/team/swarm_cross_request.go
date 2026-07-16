package team

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
)

// applyCrossRequestEntryOverride moves EntryPoint to the last active swarm member
// when CrossRequestTransfer is enabled (Graph-path equivalent of WithCrossRequestTransfer).
func applyCrossRequestEntryOverride(cfg biz.GraphBuildConfig, def Definition, activeAgentKey string) biz.GraphBuildConfig {
	if def.Swarm == nil || !def.Swarm.CrossRequestTransfer {
		return cfg
	}
	activeAgentKey = strings.TrimSpace(activeAgentKey)
	if activeAgentKey == "" {
		return cfg
	}
	for _, n := range cfg.Nodes {
		if strings.EqualFold(strings.TrimSpace(n.AgentName), activeAgentKey) {
			cfg.EntryPoint = n.ID
			return cfg
		}
		if strings.EqualFold(strings.TrimSpace(n.ID), activeAgentKey) {
			cfg.EntryPoint = n.ID
			return cfg
		}
	}
	return cfg
}

func readSwarmActiveAgent(sess biz.Session) string {
	raw := strings.TrimSpace(sess.MetadataJSON)
	if raw == "" {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return ""
	}
	v, _ := meta[biz.SwarmActiveAgentSessionMeta].(string)
	return strings.TrimSpace(v)
}

func writeSwarmActiveAgent(ctx context.Context, sessions biz.SessionTurnManager, sess biz.Session, agentKey string) {
	if sessions == nil {
		return
	}
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return
	}
	meta := map[string]any{}
	if raw := strings.TrimSpace(sess.MetadataJSON); raw != "" {
		_ = json.Unmarshal([]byte(raw), &meta)
	}
	if meta == nil {
		meta = map[string]any{}
	}
	meta[biz.SwarmActiveAgentSessionMeta] = agentKey
	b, err := json.Marshal(meta)
	if err != nil {
		return
	}
	metaJSON := string(b)
	_, _ = sessions.Update(ctx, sess.ID, session.SessionUpdateFields{MetadataJSON: &metaJSON})
}
