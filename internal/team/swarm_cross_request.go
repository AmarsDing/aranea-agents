package team

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
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
	// S6 CAS：原子更新单个 metadata key（jsonb_set），避免 read-modify-write
	// 丢失其他子系统对 metadata 的并发更新（lost update）。
	_ = sessions.UpdateMetadataKey(ctx, sess.ID, biz.SwarmActiveAgentSessionMeta, agentKey)
}
