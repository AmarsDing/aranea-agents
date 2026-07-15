package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// L3MemoryCue formats active semantic facts for prompt injection via fused multi-scope recall.
// l1FieldValues contains pinned L1 field values for cross-layer dedup — facts whose normalized
// statement matches any L1 value are filtered out to avoid redundancy.
func L3MemoryCue(ctx context.Context, l3 biz.MemoryL3Recaller, ag biz.Agent, policy biz.MemoryRuntimePolicy, rt biz.MemoryRuntimeContext, keyword string, limit int, l1FieldValues []string, lg loggateway.Logger) string {
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
	if err != nil {
		lg.Warn("L3 memory query failed", loggateway.StepID("agent.memory_query_fail"), loggateway.Err(err))
		return ""
	}
	if len(rows) == 0 {
		return ""
	}

	// Cross-layer dedup: filter out L3 facts whose statement matches an L1 field value.
	rows = biz.DedupL3WithL1(rows, l1FieldValues)
	if len(rows) == 0 {
		return ""
	}

	maxChars := policy.L3MaxPerRecallChars
	type l3Entry struct {
		stmt      string
		factID    string
		srcSess   string
		confidence float64
		version   int
	}
	var entries []l3Entry
	for _, raw := range rows {
		if len(entries) >= limit {
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
		e := l3Entry{stmt: stmt}
		if policy.L3InjectProvenance {
			e.factID = strings.TrimSpace(fmt.Sprint(row["id"]))
			e.srcSess = strings.TrimSpace(fmt.Sprint(row["source_session_id"]))
			if c, ok := row["confidence"].(float64); ok {
				e.confidence = c
			}
			if v, ok := row["version"].(float64); ok {
				e.version = int(v)
			}
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## L3 semantic memory (user facts)\n")
	b.WriteString("The following facts were learned from prior conversations. Use when relevant; do not invent beyond them.\n")
	for _, e := range entries {
		if policy.L3InjectProvenance && e.factID != "" && e.factID != "<nil>" {
			suffix := formatL3Provenance(e.factID, e.srcSess, e.confidence, e.version)
			fmt.Fprintf(&b, "- %s%s\n", e.stmt, suffix)
		} else {
			fmt.Fprintf(&b, "- %s\n", e.stmt)
		}
	}
	return strings.TrimSpace(b.String())
}

// formatL3Provenance builds a compact provenance suffix for an L3 fact.
// Example: " [id:abc123, src:sess-1, conf:0.85, v3]"
func formatL3Provenance(factID, srcSess string, confidence float64, version int) string {
	var b strings.Builder
	b.WriteString(" [")
	// Shorten the fact ID to the first 8 chars for compactness.
	shortID := factID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	fmt.Fprintf(&b, "id:%s", shortID)
	if srcSess != "" && srcSess != "<nil>" {
		shortSrc := srcSess
		if len(shortSrc) > 8 {
			shortSrc = shortSrc[:8]
		}
		fmt.Fprintf(&b, ", src:%s", shortSrc)
	}
	if confidence > 0 {
		fmt.Fprintf(&b, ", conf:%.2f", confidence)
	}
	if version > 0 {
		fmt.Fprintf(&b, ", v%d", version)
	}
	b.WriteByte(']')
	return b.String()
}
