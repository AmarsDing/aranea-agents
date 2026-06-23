package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
)

// VersionHashEntry is a sortable id:timestamp pair used for content-based hashing.
type VersionHashEntry struct {
	ID        string
	UpdatedAt string
}

// ComputeVersionHash produces a SHA-256 hex digest from a set of id:timestamp pairs.
// Entries are sorted by ID before hashing so the result is deterministic regardless
// of insertion order. An empty slice yields an empty string (no hash contribution).
func ComputeVersionHash(entries []VersionHashEntry) string {
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

// ComputeToolVersionHash produces a content hash from an effective-tools result.
func ComputeToolVersionHash(eff *biz.AgentEffectiveTools) string {
	if eff == nil {
		return ""
	}
	entries := make([]VersionHashEntry, 0, len(eff.Items))
	for _, item := range eff.Items {
		state := "0"
		if item.Enabled {
			state = "1"
		}
		entries = append(entries, VersionHashEntry{
			ID:        fmt.Sprintf("%s:%s", item.ToolKey, state),
			UpdatedAt: item.EffectiveState,
		})
	}
	return ComputeVersionHash(entries)
}

// ComputeSkillVersionHash produces a content hash from a list of enabled skill slugs.
func ComputeSkillVersionHash(slugs []string) string {
	if len(slugs) == 0 {
		return ""
	}
	entries := make([]VersionHashEntry, len(slugs))
	for i, slug := range slugs {
		entries[i] = VersionHashEntry{ID: slug}
	}
	return ComputeVersionHash(entries)
}

// ComputeMCPVersionHash produces a content hash from a list of effective MCP servers.
func ComputeMCPVersionHash(servers []biz.EffectiveMCPServer) string {
	if len(servers) == 0 {
		return ""
	}
	entries := make([]VersionHashEntry, len(servers))
	for i, s := range servers {
		entries[i] = VersionHashEntry{ID: s.ID, UpdatedAt: s.ConfigJSON}
	}
	return ComputeVersionHash(entries)
}
