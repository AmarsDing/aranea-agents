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
	cue, _ := L3MemoryCueWithIDs(ctx, l3, ag, policy, rt, keyword, limit, l1FieldValues, lg)
	return cue
}

// L3MemoryCueWithIDs is L3MemoryCue plus the IDs of the facts actually
// written into the cue (FR-12.6: the before-model hook increments
// injected_count for exactly this set once per turn).
func L3MemoryCueWithIDs(ctx context.Context, l3 biz.MemoryL3Recaller, ag biz.Agent, policy biz.MemoryRuntimePolicy, rt biz.MemoryRuntimeContext, keyword string, limit int, l1FieldValues []string, lg loggateway.Logger) (string, []string) {
	if l3 == nil || !policy.InjectL3 {
		return "", nil
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
		return "", nil
	}
	if len(rows) == 0 {
		return "", nil
	}

	// Cross-layer dedup: filter out L3 facts whose statement matches an L1 field value.
	rows = biz.DedupL3WithL1(rows, l1FieldValues)
	if len(rows) == 0 {
		return "", nil
	}

	maxChars := policy.L3MaxPerRecallChars
	type l3Entry struct {
		stmt       string
		id         string
		srcSess    string
		confidence float64
		version    int
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
		if id := strings.TrimSpace(fmt.Sprint(row["id"])); id != "" && id != "<nil>" {
			e.id = id
		}
		if policy.L3InjectProvenance {
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
		return "", nil
	}
	var b strings.Builder
	header := "## L3 semantic memory (user facts)\n" +
		"The following facts were learned from prior conversations. Use when relevant; do not invent beyond them.\n"
	b.WriteString(header)
	// FR-12/P2: pack lines into the recall-block token budget (score-descended
	// input, "按分截断"); factIDs collect only KEPT facts so injected_count
	// counts exactly what entered the prompt.
	packer := newRecallLinePacker(policy.L3RecallBudgetTokens)
	packer.allow(header)
	var factIDs []string
	for _, e := range entries {
		var line string
		if policy.L3InjectProvenance && e.id != "" {
			line = fmt.Sprintf("- %s%s", e.stmt, formatL3Provenance(e.id, e.srcSess, e.confidence, e.version))
		} else {
			line = fmt.Sprintf("- %s", e.stmt)
		}
		if !packer.allow(line) {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if e.id != "" {
			factIDs = append(factIDs, e.id)
		}
	}
	if len(factIDs) == 0 && b.Len() <= len(header) {
		return "", nil
	}
	return strings.TrimSpace(b.String()), factIDs
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
