package biz

import (
	"context"
	"encoding/json"
	"strconv"

	"aranea-agents/pkg/loggateway"
)

// ── 计数归因（P1）───────────────────────────────────────────────────────────
//
// 回答「上一次改进有没有用」：新一周期 draft 生成前，对最近一次 applied
// 建议做有效性裁决（helpful/harmful/neutral/insufficient_data），裁决回写
// 到该建议的 effectiveness metadata（幂等），并从其 delta_ops 提取
// AffectedRuleIDs 供计数归账（BumpRuleCounters）。

const (
	// attributionMinInvocations 是裁决所需的最小调用次数；低于此值数据不足。
	attributionMinInvocations = 5
	// attributionDeltaThreshold 是成功率变化的裁决阈值（±5 个百分点）。
	attributionDeltaThreshold = 0.05
)

// EvolutionAttribution is the effectiveness verdict for the most recent
// applied evolution of a skill, plus the rules it touched.
type EvolutionAttribution struct {
	Verdict             string // helpful | harmful | neutral | insufficient_data
	BaselineSuccessRate float64
	CurrentSuccessRate  float64
	// AffectedRuleIDs are the rule IDs touched by the last applied delta
	// (empty for full-rewrite mode evolutions).
	AffectedRuleIDs []string
}

// AttributeLastEvolution computes the effectiveness verdict for the most
// recent applied evolution suggestion of the given skill. Returns nil when
// there is no applied suggestion (first evolution cycle) or the verdict
// cannot be computed (missing baseline / store unavailable / read failure —
// all nil-safe degradations, the evolution loop proceeds without attribution).
//
// Side effect: the verdict is written back to the applied suggestion's
// effectiveness metadata key (idempotent — already-set verdicts are kept).
func (uc *SkillIntelligenceUsecase) AttributeLastEvolution(ctx context.Context, skillID string) *EvolutionAttribution {
	if uc.unifiedStore == nil || uc.aggregator == nil {
		return nil
	}
	applied, err := uc.unifiedStore.ListByTarget(ctx, string(EvolutionTargetSkill), skillID, "applied", 1, 0)
	if err != nil || len(applied) == 0 {
		return nil
	}
	last := &applied[0]

	// Already adjudicated → reuse the stored verdict without recomputing.
	if existing := last.MetaString(EvoMetaEffectiveness); existing != "" {
		return &EvolutionAttribution{
			Verdict:         existing,
			AffectedRuleIDs: parseAffectedRuleIDs(last),
		}
	}

	// Baseline is required for any verdict. Written at Create time as a proper
	// JSON number; tolerate the JSON-string form defensively.
	baseline, ok := metaFloat(last, EvoMetaBaselineSuccessRate)
	if !ok {
		return nil
	}

	// Compare against invocations since the suggestion was applied.
	since := last.CreatedAt
	if last.AppliedAt != nil {
		since = *last.AppliedAt
	}
	metrics, err := uc.aggregator.GetHealthMetrics(ctx, skillID, since)
	if err != nil {
		uc.lg.Warn("AttributeLastEvolution: GetHealthMetrics failed",
			loggateway.StepID("skill_intelligence.attribution"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil
	}

	attr := &EvolutionAttribution{
		BaselineSuccessRate: baseline,
		CurrentSuccessRate:  metrics.SuccessRate,
		AffectedRuleIDs:     parseAffectedRuleIDs(last),
	}
	switch {
	case metrics.InvocationCount < attributionMinInvocations:
		attr.Verdict = EvoEffectivenessInsufficientData
	case metrics.SuccessRate-baseline >= attributionDeltaThreshold:
		attr.Verdict = EvoEffectivenessHelpful
	case metrics.SuccessRate-baseline <= -attributionDeltaThreshold:
		attr.Verdict = EvoEffectivenessHarmful
	default:
		attr.Verdict = EvoEffectivenessNeutral
	}

	// Write back the verdict (idempotent guard above makes re-entry safe).
	if wErr := uc.unifiedStore.UpdateMetadataKey(ctx, last.ID, EvoMetaEffectiveness, attr.Verdict); wErr != nil {
		uc.lg.Warn("AttributeLastEvolution: write back effectiveness failed",
			loggateway.StepID("skill_intelligence.attribution"),
			loggateway.Str("suggestion_id", last.ID),
			loggateway.Err(wErr))
	}
	return attr
}

// parseAffectedRuleIDs extracts the rule IDs touched by a suggestion's stored
// delta ops. Returns nil for full-rewrite mode suggestions (no delta_ops key).
// delta_ops is persisted via UpdateMetadataKey, which stores values as JSON
// strings — hence MetaString + second unmarshal.
func parseAffectedRuleIDs(s *UnifiedEvolutionSuggestion) []string {
	raw := s.MetaString(EvoMetaDeltaOps)
	if raw == "" {
		return nil
	}
	var ops []DeltaOp
	if err := json.Unmarshal([]byte(raw), &ops); err != nil {
		return nil
	}
	seen := make(map[string]struct{}, len(ops))
	ids := make([]string, 0, len(ops))
	for _, op := range ops {
		if _, dup := seen[op.RuleID]; dup {
			continue
		}
		seen[op.RuleID] = struct{}{}
		ids = append(ids, op.RuleID)
	}
	return ids
}

// metaFloat reads a metadata key as a float64, tolerating both the JSON
// number form (written at Create time) and the JSON-string form (written via
// UpdateMetadataKey).
func metaFloat(s *UnifiedEvolutionSuggestion, key string) (float64, bool) {
	if raw := s.MetaRaw(key); len(raw) > 0 {
		if f, err := strconv.ParseFloat(string(raw), 64); err == nil {
			return f, true
		}
	}
	if str := s.MetaString(key); str != "" {
		if f, err := strconv.ParseFloat(str, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}
