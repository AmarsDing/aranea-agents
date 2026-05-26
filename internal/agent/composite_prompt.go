package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// CompositeMemoryCue formats fused L2+L3 recall as one prompt block sorted by score.
func CompositeMemoryCue(ctx context.Context, composite biz.MemoryCompositeRecaller, ag biz.Agent, policy biz.MemoryRuntimePolicy, rt biz.MemoryRuntimeContext, sessionID, keyword string, limit int) string {
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
	if err != nil || len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## L2+L3 memory (fused recall)\n")
	b.WriteString("Episodes and semantic facts ranked together. Use when relevant; do not invent beyond them.\n")
	for i, hit := range hits {
		if i >= limit {
			break
		}
		line := strings.TrimSpace(hit.Line)
		if line == "" {
			continue
		}
		if policy.InjectL3 && policy.L3MaxPerRecallChars > 0 && len(line) > policy.L3MaxPerRecallChars {
			line = line[:policy.L3MaxPerRecallChars] + "…"
		}
		prefix := strings.ToUpper(strings.TrimSpace(hit.Layer))
		if prefix == "" {
			prefix = "MEM"
		}
		fmt.Fprintf(&b, "- [%s] %s\n", prefix, line)
	}
	return strings.TrimSpace(b.String())
}
