package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// L3MemoryCue formats active semantic facts for prompt injection.
func L3MemoryCue(ctx context.Context, l3 biz.MemoryL3Recaller, ag biz.Agent, userID, keyword string, limit int) string {
	if l3 == nil || ag.Settings == nil || !ag.Settings.L3Enabled || !ag.Settings.L0InjectL3 {
		return ""
	}
	agentID := strings.TrimSpace(ag.ID)
	if agentID == "" {
		return ""
	}
	if limit <= 0 {
		limit = 12
	}
	if limit > 20 {
		limit = 20
	}
	rows, err := l3.RecallFacts(ctx, biz.L3RecallQuery{
		ScopeType: "agent",
		ScopeID:   agentID,
		UserID:    userID,
		Query:     strings.TrimSpace(keyword),
		Limit:     int32(limit),
	})
	if err != nil || len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## L3 semantic memory (user facts)\n")
	b.WriteString("The following facts were learned from prior conversations. Use when relevant; do not invent beyond them.\n")
	for i, raw := range rows {
		if i >= limit {
			break
		}
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		stmt := strings.TrimSpace(fmt.Sprint(row["statement"]))
		if stmt == "" || stmt == "<nil>" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", stmt)
	}
	return strings.TrimSpace(b.String())
}
