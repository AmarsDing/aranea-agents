package service

import (
	"strings"

	chatagent "aranea-agents/internal/agent"
)

func invalidateAgentBuildCache(agentID string) {
	id := strings.TrimSpace(agentID)
	if id == "" {
		return
	}
	chatagent.InvalidateAgentCache(id)
}
