package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

func memberAgentKeys(
	ctx context.Context,
	def Definition,
	lookupAgent func(ctx context.Context, id string) (biz.Agent, error),
) ([]string, error) {
	members := EnabledMembers(def)
	keys := make([]string, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, m := range members {
		ag, err := lookupAgent(ctx, strings.TrimSpace(m.AgentID))
		if err != nil {
			return nil, err
		}
		key := strings.TrimSpace(ag.AgentKey)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}
