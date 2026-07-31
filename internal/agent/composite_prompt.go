package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
)

// Pinned preference injection (FR-M3): active preference/constraint facts are
// always injected, bypassing vector scoring.
const (
	// pinnedPreferenceMax caps the number of pinned facts per turn.
	pinnedPreferenceMax = 10
	// pinnedPreferenceItemMaxRunes caps one pinned statement length.
	pinnedPreferenceItemMaxRunes = 200
)

// pinnedPreferenceKinds are the fact kinds eligible for pinned injection.
var pinnedPreferenceKinds = []string{"preference", "constraint"}

// PinnedPreferenceCue renders the always-on preference/constraint block
// (FR-M3). Returns "" when the lister is nil, errors, or yields no usable
// rows — pinned injection is best-effort and must never break a turn.
func PinnedPreferenceCue(ctx context.Context, lister biz.MemoryPreferenceLister, agentID, userID string) string {
	if lister == nil {
		return ""
	}
	rows, err := lister.ListActivePreferenceFacts(ctx, agentID, userID, pinnedPreferenceKinds, pinnedPreferenceMax)
	if err != nil || len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## 用户偏好与工作要求（始终生效）\n")
	written := 0
	for _, raw := range rows {
		if written >= pinnedPreferenceMax {
			break
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		stmt, _ := m["statement"].(string)
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if runes := []rune(stmt); len(runes) > pinnedPreferenceItemMaxRunes {
			stmt = string(runes[:pinnedPreferenceItemMaxRunes]) + "…"
		}
		kind, _ := m["fact_kind"].(string)
		prefix := "PREFERENCE"
		if kind == "constraint" {
			prefix = "CONSTRAINT"
		}
		fmt.Fprintf(&b, "- [%s] %s\n", prefix, stmt)
		written++
	}
	if written == 0 {
		return ""
	}
	return strings.TrimSpace(b.String())
}

// CompositeMemoryCue formats fused L2+L3 recall as one prompt block sorted by score.
// proactiveHits are optional results from ProactiveRecall (P3-11) that are merged
// with RecallComposite results, deduplicated by line, and ranked by score.
func CompositeMemoryCue(ctx context.Context, composite biz.MemoryCompositeRecaller, ag biz.Agent, policy biz.MemoryRuntimePolicy, rt biz.MemoryRuntimeContext, sessionID, keyword string, limit int, proactiveHits []biz.CompositeRecallHit) string {
	if composite == nil || !policy.RecallL2 || !policy.InjectL3 {
		return ""
	}
	agentID := strings.TrimSpace(ag.ID)
	if agentID == "" {
		return ""
	}
	if limit <= 0 {
		limit = policy.L2RecallMax + policy.L3RecallTopK
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	hits, err := composite.RecallComposite(ctx, biz.CompositeRecallQuery{
		AgentID:   agentID,
		SessionID: sessionID,
		UserID:    rt.UserID,
		Query:     strings.TrimSpace(keyword),
		Limit:     int32(limit),
	})
	if err != nil {
		hits = nil
	}
	merged := mergeCompositeHits(hits, proactiveHits, limit)
	if len(merged) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## L2+L3 memory (fused recall)\n")
	b.WriteString("Episodes and semantic facts ranked together. Use when relevant; do not invent beyond them.\n")
	for i, hit := range merged {
		if i >= limit {
			break
		}
		line := strings.TrimSpace(hit.Line)
		if line == "" {
			continue
		}
		if policy.InjectL3 && policy.L3MaxPerRecallChars > 0 && len([]rune(line)) > policy.L3MaxPerRecallChars {
			line = string([]rune(line)[:policy.L3MaxPerRecallChars]) + "…"
		}
		prefix := strings.ToUpper(strings.TrimSpace(hit.Layer))
		if prefix == "" {
			prefix = "MEM"
		}
		fmt.Fprintf(&b, "- [%s] %s", prefix, line)
		// P2-04: append provenance for L3 facts when available.
		if policy.L3InjectProvenance && hit.Layer == "L3" && hit.FactID != "" {
			b.WriteString(formatL3Provenance(hit.FactID, hit.SourceSession, hit.Confidence, hit.Version))
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// mergeCompositeHits deduplicates recall and proactive hits by line (case-insensitive)
// and ranks them by score descending. Proactive hits are appended after recall hits
// so that explicit keyword recall takes precedence on ties (stable sort).
func mergeCompositeHits(recallHits, proactiveHits []biz.CompositeRecallHit, limit int) []biz.CompositeRecallHit {
	if len(recallHits) == 0 && len(proactiveHits) == 0 {
		return nil
	}
	if len(proactiveHits) == 0 {
		if limit > 0 && len(recallHits) > limit {
			return recallHits[:limit]
		}
		return recallHits
	}
	seen := make(map[string]bool, len(recallHits)+len(proactiveHits))
	merged := make([]biz.CompositeRecallHit, 0, len(recallHits)+len(proactiveHits))
	for _, h := range recallHits {
		key := strings.ToLower(strings.TrimSpace(h.Line))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, h)
	}
	for _, h := range proactiveHits {
		key := strings.ToLower(strings.TrimSpace(h.Line))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, h)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
