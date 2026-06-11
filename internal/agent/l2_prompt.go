package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// L2MemoryCue formats episodic memories for prompt injection (keyword/vector rerank when query set).
func L2MemoryCue(ctx context.Context, l2 biz.MemoryL2Recaller, ag biz.Agent, policy biz.MemoryRuntimePolicy, sessionID, query string, limit int, lg loggateway.Logger) string {
	if l2 == nil || !policy.RecallL2 {
		return ""
	}
	agentID := strings.TrimSpace(ag.ID)
	if agentID == "" {
		return ""
	}
	if limit <= 0 {
		limit = policy.L2RecallMax
	}
	if limit <= 0 {
		limit = 3
	}
	if limit > 10 {
		limit = 10
	}
	rows, err := l2.RecallEpisodes(ctx, biz.L2RecallQuery{
		AgentID:   agentID,
		SessionID: sessionID,
		Query:     strings.TrimSpace(query),
		Limit:     int32(limit),
	})
	if err != nil {
		lg.Warn("L2 memory query failed", loggateway.StepID("agent.memory_query_fail"), loggateway.Err(err))
		return ""
	}
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## L2 episodic memory (recent sessions)\n")
	b.WriteString("Summaries of prior consolidated interactions. Use for continuity when relevant.\n")
	for i, raw := range rows {
		if i >= limit {
			break
		}
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		title := strings.TrimSpace(fmt.Sprint(row["title"]))
		summary := strings.TrimSpace(fmt.Sprint(row["outcome_summary"]))
		if title == "" || title == "<nil>" {
			title = summary
		}
		if title == "" || title == "<nil>" {
			continue
		}
		if summary != "" && summary != title && summary != "<nil>" {
			fmt.Fprintf(&b, "- %s: %s\n", title, summary)
		} else {
			fmt.Fprintf(&b, "- %s\n", title)
		}
	}
	return strings.TrimSpace(b.String())
}
