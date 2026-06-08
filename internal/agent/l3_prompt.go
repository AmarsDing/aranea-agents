package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// L3MemoryCue formats active semantic facts for prompt injection via fused multi-scope recall.
// l1FieldValues contains pinned L1 field values for cross-layer dedup — facts whose normalized
// statement matches any L1 value are filtered out to avoid redundancy.
func L3MemoryCue(ctx context.Context, l3 biz.MemoryL3Recaller, ag biz.Agent, policy biz.MemoryRuntimePolicy, rt biz.MemoryRuntimeContext, keyword string, limit int, l1FieldValues []string) string {
	if l3 == nil || !policy.InjectL3 {
		return ""
	}
	if limit <= 0 {
		limit = policy.L3RecallTopK
	}
	rows, err := l3.RecallFactsFused(ctx, biz.L3FusedRecallQuery{
		Runtime:         rt,
		Scopes:          policy.L3RecallScopes,
		Query:           strings.TrimSpace(keyword),
		Limit:           int32(limit),
		MinScoreQuery:   policy.L3MinScoreQuery,
		MinScorePassive: policy.L3MinScorePassive,
	})
	if err != nil || len(rows) == 0 {
		return ""
	}

	// Cross-layer dedup: filter out L3 facts whose statement matches an L1 field value.
	rows = biz.DedupL3WithL1(rows, l1FieldValues)
	if len(rows) == 0 {
		return ""
	}

	maxChars := policy.L3MaxPerRecallChars
	var statements []string
	for _, raw := range rows {
		if len(statements) >= limit {
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
		if len(stmt) > maxChars {
			stmt = safeTruncate(stmt, maxChars)
		}
		statements = append(statements, stmt)
	}
	if len(statements) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## L3 semantic memory (user facts)\n")
	b.WriteString("The following facts were learned from prior conversations. Use when relevant; do not invent beyond them.\n")
	for _, stmt := range statements {
		fmt.Fprintf(&b, "- %s\n", stmt)
	}
	return strings.TrimSpace(b.String())
}
