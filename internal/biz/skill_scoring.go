package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// Scoring and analysis thresholds.
const (
	TimeoutThresholdMS       = 30000 // Duration above which a skill invocation is considered timed out
	ContextOverflowThreshold = 5000  // Input preview length above which context overflow is suspected
	MinInvocationCount       = 5     // Minimum invocations needed for a reliable score
	DefaultNeutralScore      = 50    // Default score when insufficient data is available
)

// Failure tag constants for ExperienceReport.FailureTags.
const (
	FailureTagParamMismatch        = "param_mismatch"
	FailureTagWrongToolChoice      = "wrong_tool_choice"
	FailureTagToolTimeout          = "tool_timeout"
	FailureTagToolAPIError         = "tool_api_error"
	FailureTagContextOverflow      = "context_overflow"
	FailureTagInstructionAmbiguity = "instruction_ambiguity"
	FailureTagUserCancelled        = "user_cancelled"
	FailureTagUnknown              = "unknown"
)

// ScoreWeights holds configurable weights for the skill scoring model.
type ScoreWeights struct {
	SuccessRate float64 `json:"success_rate"`
	Duration    float64 `json:"duration"`
	Token       float64 `json:"token"`
	Feedback    float64 `json:"feedback"`
}

// DefaultScoreWeights returns the v1 default scoring weights.
func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{
		SuccessRate: 0.4,
		Duration:    0.25,
		Token:       0.2,
		Feedback:    0.15,
	}
}

// SkillScoringUsecase handles skill invocation analysis and scoring.
// Extracted from SkillIntelligenceUsecase to reduce cognitive complexity (AS-COG-01).
type SkillScoringUsecase struct {
	aggregator SkillHealthAggregator
	lg         loggateway.Logger
	weights    ScoreWeights
}

// NewSkillScoringUsecase constructs a SkillScoringUsecase.
func NewSkillScoringUsecase(
	aggregator SkillHealthAggregator,
	lg loggateway.Logger,
) *SkillScoringUsecase {
	return &SkillScoringUsecase{
		aggregator: aggregator,
		lg:         lg,
		weights:    DefaultScoreWeights(),
	}
}

// Compile-time interface check: SkillScoringUsecase satisfies SkillScorer.
var _ SkillScorer = (*SkillScoringUsecase)(nil)

// AnalyzeInvocation performs rule-based analysis of a skill invocation and
// returns structured fields: success/failure outcome, failure tags, and
// latency classification.
func (uc *SkillScoringUsecase) AnalyzeInvocation(ctx context.Context, inv SkillInvocationWrite) (isSuccess bool, failureTags []string) {
	isSuccess = inv.Outcome == "success"

	if isSuccess {
		return isSuccess, nil
	}

	// Rule-based failure tag extraction.
	tags := make([]string, 0, 2)

	// Timeout detection: duration > 30s with error.
	if inv.DurationMS > TimeoutThresholdMS && inv.ErrorCode != "" {
		tags = append(tags, FailureTagToolTimeout)
	}

	// API error detection.
	if inv.ErrorCode != "" {
		switch {
		case strings.Contains(inv.ErrorCode, "api") || strings.Contains(inv.ErrorCode, "429") || strings.Contains(inv.ErrorCode, "500"):
			tags = append(tags, FailureTagToolAPIError)
		case strings.Contains(inv.ErrorCode, "param") || strings.Contains(inv.ErrorCode, "invalid"):
			tags = append(tags, FailureTagParamMismatch)
		}
	}

	// Context overflow: very long input + error.
	if len(inv.InputPreview) > ContextOverflowThreshold && inv.Outcome == "failure" {
		tags = append(tags, FailureTagContextOverflow)
	}

	// Wrong tool choice: error message mentions wrong/incorrect/unexpected.
	errMsgLower := strings.ToLower(inv.ErrorMessage)
	if strings.Contains(errMsgLower, "wrong") || strings.Contains(errMsgLower, "incorrect") || strings.Contains(errMsgLower, "unexpected") {
		tags = append(tags, FailureTagWrongToolChoice)
	}

	// Instruction ambiguity: error message mentions ambiguous/unclear/confusing.
	if strings.Contains(errMsgLower, "ambiguous") || strings.Contains(errMsgLower, "unclear") || strings.Contains(errMsgLower, "confusing") {
		tags = append(tags, FailureTagInstructionAmbiguity)
	}

	if len(tags) == 0 {
		tags = append(tags, FailureTagUnknown)
	}

	return isSuccess, tags
}

// ScoreSkill computes a weighted 0-100 score for a skill based on historical
// health metrics. Missing data items default to neutral value 0.5.
func (uc *SkillScoringUsecase) ScoreSkill(ctx context.Context, skillID string) (int, error) {
	skillID, err := requireNonEmpty(skillID, "SKILL_INTELLIGENCE", "skill_id")
	if err != nil {
		return 0, err
	}

	since30d := time.Now().UTC().Add(-30 * 24 * time.Hour)
	metrics, err := uc.aggregator.GetHealthMetrics(ctx, skillID, since30d)
	if err != nil {
		uc.lg.Warn("ScoreSkill: GetHealthMetrics failed",
			loggateway.StepID("skill_intelligence.score"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return 0, err
	}

	// If not enough data, return neutral score.
	if metrics.InvocationCount < MinInvocationCount {
		return DefaultNeutralScore, nil
	}

	// Normalize factors.
	successRateFactor := metrics.SuccessRate // already 0-1

	// Dynamic weight redistribution: omit weights for unavailable data
	// instead of using neutral 0.5 which pulls the score toward the middle.
	w := uc.weights
	effectiveSuccessW := w.SuccessRate
	effectiveDurationW := 0.0
	effectiveTokenW := 0.0
	effectiveFeedbackW := 0.0

	// Duration factor: lower is better. Normalize against a 30s baseline.
	var durationFactor float64
	if metrics.AvgDurationMS > 0 {
		effectiveDurationW = w.Duration
		durationFactor = 1.0 - normalizeDuration(metrics.AvgDurationMS)
		if durationFactor < 0 {
			durationFactor = 0
		}
	}

	// Token factor: lower is better. Normalize against a 2000-token baseline.
	var tokenFactor float64
	if metrics.AvgTokenUsage > 0 {
		effectiveTokenW = w.Token
		tokenFactor = 1.0 - normalizeTokenUsage(metrics.AvgTokenUsage)
		if tokenFactor < 0 {
			tokenFactor = 0
		}
	}

	// Feedback factor: use heuristic score if DB field not yet available.
	var feedbackFactor float64
	if metrics.FeedbackScore > 0 {
		effectiveFeedbackW = w.Feedback
		feedbackFactor = metrics.FeedbackScore
	} else {
		// TEMPORARY: heuristic feedback score until DB field is added.
		heuristicScore := computeHeuristicFeedbackScore(metrics)
		if heuristicScore > 0 {
			effectiveFeedbackW = w.Feedback
			feedbackFactor = heuristicScore
		}
	}

	totalW := effectiveSuccessW + effectiveDurationW + effectiveTokenW + effectiveFeedbackW
	if totalW == 0 {
		return DefaultNeutralScore, nil
	}

	score := (effectiveSuccessW*successRateFactor +
		effectiveDurationW*durationFactor +
		effectiveTokenW*tokenFactor +
		effectiveFeedbackW*feedbackFactor) / totalW

	result := int(score * 100)
	if result < 0 {
		result = 0
	}
	if result > 100 {
		result = 100
	}
	return result, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func normalizeDuration(avgMS float64) float64 {
	// Baseline: 30 seconds. Normalize to 0-1 range.
	const baselineMS = 30000
	if avgMS <= 0 {
		return 0
	}
	ratio := avgMS / baselineMS
	if ratio > 1 {
		return 1
	}
	return ratio
}

// normalizeTokenUsage normalizes average token usage to a 0-1 range.
// Lower is better: 0 tokens → 0, baselineTokens → 1, above baseline → 1.
func normalizeTokenUsage(avgTokens int) float64 {
	const baselineTokens = 2000
	if avgTokens <= 0 {
		return 0
	}
	ratio := float64(avgTokens) / baselineTokens
	if ratio > 1 {
		return 1
	}
	return ratio
}

// computeHeuristicFeedbackScore derives a provisional feedback score from
// available health metrics. TEMPORARY: this will be replaced by a proper
// feedback_score DB field in a future migration.
//
// Focuses on efficiency and consistency signals only — NOT success rate,
// which is already captured by successRateFactor in ScoreSkill.
func computeHeuristicFeedbackScore(m *SkillHealthMetrics) float64 {
	if m == nil || m.InvocationCount == 0 {
		return 0
	}

	var score float64

	// Duration efficiency: fast skills get bonus (max 0.2).
	if m.AvgDurationMS > 0 && m.AvgDurationMS < 5000 {
		score += 0.2
	} else if m.AvgDurationMS > 0 && m.AvgDurationMS < 15000 {
		score += 0.1
	}

	// Token efficiency: low token usage gets bonus (max 0.15).
	if m.AvgTokenUsage > 0 && m.AvgTokenUsage < 1000 {
		score += 0.15
	} else if m.AvgTokenUsage > 0 && m.AvgTokenUsage < 3000 {
		score += 0.05
	}

	// Consistency: high invocation count with good P95 indicates stable skill (max 0.3).
	if m.InvocationCount >= 10 && m.P95DurationMS > 0 && m.P95DurationMS < 10000 {
		score += 0.15
	}
	if m.InvocationCount >= 50 {
		score += 0.15
	}

	if score > 1 {
		score = 1
	}
	if score < 0 {
		score = 0
	}
	return score
}
