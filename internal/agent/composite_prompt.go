package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"
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
// G 维 P1（2026-08-18）：除 consolidation 分类法（preference/constraint）外，
// 覆盖 immediate self-marking 分类法（user_preference/agent_instruction）——
// 规则类事实与偏好同通道 100% 钉住，不再依赖相似度召回门控。
var pinnedPreferenceKinds = []string{"preference", "constraint", "user_preference", "agent_instruction"}

// PinnedPreferenceCue renders the always-on preference/constraint/rule block
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
	// P2a（2026-08-18）：钉住块合规引导——规则类条目每次作答必须遵守，
	// 不得因与当前问题相关性低而跳过；输出前逐条核对。
	b.WriteString("以下为长期生效的偏好与工作规则：每次作答前逐条核对并全部遵守，不得因与当前问题相关性低而忽略。\n")
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
		stmt = strutil.TruncateRunesEllipsis(stmt, pinnedPreferenceItemMaxRunes)
		kind, _ := m["fact_kind"].(string)
		prefix := "PREFERENCE"
		switch kind {
		case "constraint":
			prefix = "CONSTRAINT"
		case "agent_instruction":
			prefix = "RULE"
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
	content = strutil.TruncateRunesEllipsis(content, profileCardMaxRunes)
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
		// P1-1（2026-08-16）：透传 L3 作用域/质量门/team 上下文，
		// 与 standalone L3 路径（l3_prompt.go）口径一致。
		TeamID:          rt.TeamID,
		Workspace:       rt.Workspace,
		Scopes:          policy.L3RecallScopes,
		MinScoreQuery:   policy.L3MinScoreQuery,
		MinScorePassive: policy.L3MinScorePassive,
	})
	if err != nil {
		hits = nil
	}
	merged := mergeCompositeHits(hits, proactiveHits, limit, biz.QueryHasNumericIntent(keyword))
	merged = dropLifestyleHitsForTaskQuery(keyword, merged)
	if len(merged) == 0 {
		return "", nil
	}
	var b strings.Builder
	header := "## L2+L3 memory (fused recall)\n" +
		"Episodes and semantic facts ranked together. If a listed fact answers the user, you MUST use it — quote the value. Do not say you have no record / 记忆中没有 / 知识库未收录 when the fact is listed. Do not call knowledge_search for a fact already in this block. Do not invent names, report IDs, brands, phone numbers, or personal preferences beyond these lines.\n"
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
		// L2 episode 行先压成 gist：全文 summary 会吃掉共享预算、挤出 L3
		// 事实（up-03 缺陷根修）；L3 短陈述不受影响。
		if strings.EqualFold(strings.TrimSpace(hit.Layer), "L2") {
			line = capL2GistLine(line)
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
// and ranks them with the shared tiebreak ordering (biz.CompositeHitTiebreakLess,
// P0-1): score desc, then numeric/numeric-recency tiebreak within the ε window.
// Proactive hits are appended after recall hits so that explicit keyword recall
// takes precedence on full ties (stable sort).
func mergeCompositeHits(recallHits, proactiveHits []biz.CompositeRecallHit, limit int, numericIntent bool) []biz.CompositeRecallHit {
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
	less := biz.CompositeHitTiebreakLess(numericIntent)
	sort.SliceStable(merged, func(i, j int) bool {
		return less(merged[i], merged[j])
	})
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

// dropLifestyleHitsForTaskQuery drops lifestyle/preference memories when the
// user turn is a work request (S05: 高危运维召回「日料/寿司」). If the query
// itself mentions those terms, keep the hits (the user asked about them).
func dropLifestyleHitsForTaskQuery(query string, hits []biz.CompositeRecallHit) []biz.CompositeRecallHit {
	if len(hits) == 0 || !biz.HasTaskActionSignal(query) {
		return hits
	}
	q := strings.ToLower(query)
	if lifestyleMemoryLine(q) {
		return hits
	}
	out := make([]biz.CompositeRecallHit, 0, len(hits))
	for _, h := range hits {
		if lifestyleMemoryLine(strings.ToLower(h.Line)) {
			continue
		}
		out = append(out, h)
	}
	return out
}

var lifestyleMemoryMarkers = []string{
	"日料", "寿司", "聚餐", "火锅", "奶茶",
	"喜欢吃", "爱吃", "听什么歌", "什么音乐", "追剧",
	"周末去", "爱好是", "喜欢看",
}

func lifestyleMemoryLine(s string) bool {
	if s == "" {
		return false
	}
	for _, m := range lifestyleMemoryMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
