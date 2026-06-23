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

// invalidateAllAgentBuildCaches evicts every entry from the global agent build cache.
// Use this when a platform-wide resource (tool catalog, skill list, MCP servers)
// changes and potentially affects all agents.
func invalidateAllAgentBuildCaches() {
	chatagent.InvalidateAllAgentCaches()
}

// versionHashEntry is a sortable id:timestamp pair used for content-based hashing.
type versionHashEntry struct {
	ID        string
	UpdatedAt string
}

// computeVersionHash delegates to the shared agent package implementation so
// chat, team, and graph paths all produce identical cache fingerprints.
func computeVersionHash(entries []versionHashEntry) string {
	if len(entries) == 0 {
		return ""
	}
	shared := make([]chatagent.VersionHashEntry, len(entries))
	for i, e := range entries {
		shared[i] = chatagent.VersionHashEntry{ID: e.ID, UpdatedAt: e.UpdatedAt}
	}
	return chatagent.ComputeVersionHash(shared)
}
