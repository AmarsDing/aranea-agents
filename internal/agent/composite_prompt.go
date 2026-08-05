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
	cue, _ := PinnedPreferenceCueWithIDs(ctx, lister, agentID, userID)
	return cue
}

// PinnedPreferenceCueWithIDs is PinnedPreferenceCue plus the IDs of the
// pinned facts actually written into the cue (FR-12.6: the before-model hook
// increments injected_count for exactly this set once per turn — pinned facts
// are injected by definition).
func PinnedPreferenceCueWithIDs(ctx context.Context, lister biz.MemoryPreferenceLister, agentID, userID string) (string, []string) {
	if lister == nil {
		return "", nil
	}
	rows, err := lister.ListActivePreferenceFacts(ctx, agentID, userID, pinnedPreferenceKinds, pinnedPreferenceMax)
	if err != nil || len(rows) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("## 用户偏好与工作要求（始终生效）\n")
	written := 0
	var factIDs []string
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
		if id, _ := m["id"].(string); strings.TrimSpace(id) != "" {
			factIDs = append(factIDs, strings.TrimSpace(id))
		}
		written++
	}
	if written == 0 {
		return "", nil
	}
	return strings.TrimSpace(b.String()), factIDs
}

// profileCardMaxRunes caps the resident profile card block (FR-12.7). The
// distiller already targets a compact card; this is a hard safety net.
const profileCardMaxRunes = 1200

// ProfileCardCue renders the resident profile card block (FR-12.7): one
// distilled card per (agent, user), maintained by Sleep-time, injected
// unconditionally at the first memory-block position when L3 injection is
// enabled. Returns "" when no card exists — best-effort, never breaks a turn.
func ProfileCardCue(ctx context.Context, reader biz.MemoryProfileCardReader, agentID, userID string) string {
	if reader == nil {
		return ""
	}
	card, err := reader.GetProfileCard(ctx, agentID, userID)
	if err != nil || card == nil {
		return ""
	}
	content := strings.TrimSpace(card.Content)
	if content == "" {
		return ""
	}
	if runes := []rune(content); len(runes) > profileCardMaxRunes {
		content = string(runes[:profileCardMaxRunes]) + "…"
	}
	return "## 用户档案（长期记忆摘要，始终生效）\n" + content
}

// CompositeMemoryCue formats fused L2+L3 recall as one prompt block sorted by score.
// proactiveHits are optional results from ProactiveRecall (P3-11) that are merged
// with RecallComposite results, deduplicated by line, and ranked by score.
func CompositeMemoryCue(ctx context.Context, composite biz.MemoryCompositeRecaller, ag biz.Agent, policy biz.MemoryRuntimePolicy, rt biz.MemoryRuntimeContext, sessionID, keyword string, limit int, proactiveHits []biz.CompositeRecallHit) string {
	cue, _ := CompositeMemoryCueWithHits(ctx, composite, ag, policy, rt, sessionID, keyword, limit, proactiveHits)
	return cue
}

// CompositeMemoryCueWithHits is CompositeMemoryCue plus the merged, deduplicated
// hit list. The hits power recall-transparency events (R4): the chat UI shows
// which memories were injected into the prompt for a turn.
func CompositeMemoryCueWithHits(ctx context.Context, composite biz.MemoryCompositeRecaller, ag biz.Agent, policy biz.MemoryRuntimePolicy, rt biz.MemoryRuntimeContext, sessionID, keyword string, limit int, proactiveHits []biz.CompositeRecallHit) (string, []biz.CompositeRecallHit) {
	if composite == nil || !policy.RecallL2 || !policy.InjectL3 {
		return "", nil
	}
	agentID := strings.TrimSpace(ag.ID)
	if agentID == "" {
		return "", nil
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
		return "", nil
	}
	var b strings.Builder
	header := "## L2+L3 memory (fused recall)\n" +
		"Episodes and semantic facts ranked together. Use when relevant; do not invent beyond them.\n"
	b.WriteString(header)
	// FR-12/P2: pack lines into the recall-block token budget (header counts
	// against it). Hits arrive score-descended; only kept hits are returned so
	// RecallHits / injected_count reflect what actually entered the prompt.
	packer := newRecallLinePacker(policy.L3RecallBudgetTokens)
	packer.allow(header)
	var kept []biz.CompositeRecallHit
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
		var lb strings.Builder
		fmt.Fprintf(&lb, "- [%s] %s", prefix, line)
		// P2-04: append provenance for L3 facts when available.
		if policy.L3InjectProvenance && hit.Layer == "L3" && hit.FactID != "" {
			lb.WriteString(formatL3Provenance(hit.FactID, hit.SourceSession, hit.Confidence, hit.Version))
		}
		if !packer.allow(lb.String()) {
			continue
		}
		b.WriteString(lb.String())
		b.WriteByte('\n')
		kept = append(kept, hit)
	}
	if len(kept) == 0 {
		return "", nil
	}
	return strings.TrimSpace(b.String()), kept
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
