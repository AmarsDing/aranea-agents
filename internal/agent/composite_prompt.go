package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
)

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
		fmt.Fprintf(&b, "- [%s] %s\n", prefix, line)
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
