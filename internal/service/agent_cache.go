package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
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

// computeVersionHash produces a SHA-256 hex digest from a set of id:updatedAt pairs.
// Entries are sorted by ID before hashing so the result is deterministic regardless
// of insertion order. An empty slice yields an empty string (no hash contribution).
func computeVersionHash(entries []versionHashEntry) string {
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.ID)
		b.WriteByte(':')
		b.WriteString(e.UpdatedAt)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
