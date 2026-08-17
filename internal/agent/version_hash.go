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

// ComputeSkillVersionHash produces a content hash from enabled skill refs.
// The ref carries the skill's content version marker (platform_skill.updated_at),
// so the hash changes exactly when skill content changes and stays stable
// otherwise — no spurious rebuilds, no stale skill content.
func ComputeSkillVersionHash(refs []biz.SkillEnabledRef) string {
	if len(refs) == 0 {
		return ""
	}
	entries := make([]VersionHashEntry, len(refs))
	for i, ref := range refs {
		entries[i] = VersionHashEntry{ID: ref.Slug, UpdatedAt: ref.UpdatedAt}
	}
	return ComputeVersionHash(entries)
}

// ComputeMCPVersionHash produces a content hash from a list of effective MCP servers.
// The hash covers ID + ConfigJSON (decrypted effective config: URL/transport/timeout/
// static headers all live inside it) and additionally folds ServerKey into the entry
// id — server_key IS build-effective (MCP ToolSet name = server key, which prefixes
// every tool runtime name) yet lives outside ConfigJSON, so a key rename must bump
// the hash or cached builds would keep serving stale tool names.
//
// 契约（P0-2B 摘除 MCP CRUD 全量失效后的唯一惰性失效依据）：EffectiveMCPServer
// 新增任何「构建期消费」的字段时，必须同步折入本哈希，否则该变更将静默不生效
// （调用期注入的字段，如用户凭证 HeaderInjector，除外）。
func ComputeMCPVersionHash(servers []biz.EffectiveMCPServer) string {
	if len(servers) == 0 {
		return ""
	}
	entries := make([]VersionHashEntry, len(servers))
	for i, s := range servers {
		entries[i] = VersionHashEntry{ID: s.ID + "|" + s.ServerKey, UpdatedAt: s.ConfigJSON}
	}
	return ComputeVersionHash(entries)
}
