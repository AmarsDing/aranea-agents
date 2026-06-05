package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// Scoring and analysis thresholds.
const (
	TimeoutThresholdMS       = 30000 // Duration above which a skill invocation is considered timed out
	ContextOverflowThreshold = 5000  // Input preview length above which context overflow is suspected
	MinInvocationCount       = 5     // Minimum invocations needed for a reliable score
	DefaultNeutralScore      = 50    // Default score when insufficient data is available
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

// SkillIntelligenceUsecase provides skill intelligence analysis: invocation
// analysis, scoring, and experience report generation.
type SkillIntelligenceUsecase struct {
	writer     ExperienceReportWriter
	reader     ExperienceReportReader
	aggregator SkillHealthAggregator
	lg         loggateway.Logger
	weights    ScoreWeights
}

// NewSkillIntelligenceUsecase constructs a SkillIntelligenceUsecase.
func NewSkillIntelligenceUsecase(
	reader ExperienceReportReader,
	writer ExperienceReportWriter,
	aggregator SkillHealthAggregator,
	lg loggateway.Logger,
) *SkillIntelligenceUsecase {
	return &SkillIntelligenceUsecase{
		writer:     writer,
		reader:     reader,
		aggregator: aggregator,
		lg:         lg,
		weights:    DefaultScoreWeights(),
	}
}

// SkillIntelligenceRepo is the combined interface for skill intelligence data access.
type SkillIntelligenceRepo interface {
	ExperienceReportReader
	ExperienceReportWriter
	SkillHealthAggregator
}

// AnalyzeInvocation performs rule-based analysis of a skill invocation and
// returns structured fields: success/failure outcome, failure tags, and
// latency classification.
func (uc *SkillIntelligenceUsecase) AnalyzeInvocation(ctx context.Context, inv SkillInvocationWrite) (isSuccess bool, failureTags []string) {
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
func (uc *SkillIntelligenceUsecase) ScoreSkill(ctx context.Context, skillID string) (int, error) {
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

	// Duration factor: lower is better. Normalize against a 30s baseline.
	durationFactor := 1.0 - normalizeDuration(metrics.AvgDurationMS)
	if durationFactor < 0 {
		durationFactor = 0
	}

	// Token factor: not available from health metrics alone, use neutral.
	tokenFactor := 0.5

	// Feedback: not available yet, use neutral.
	feedbackFactor := 0.5

	// Redistribute feedback weight if no feedback data.
	w := uc.weights
	effectiveTokenW := w.Token
	effectiveFeedbackW := w.Feedback
	totalW := w.SuccessRate + w.Duration + effectiveTokenW + effectiveFeedbackW

	score := (w.SuccessRate*successRateFactor +
		w.Duration*durationFactor +
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

// GenerateReport generates an ExperienceReport for a skill invocation.
// It uses rule-based extraction for structured fields and can optionally
// use LLM for natural language summaries (degradable to rule-based only).
func (uc *SkillIntelligenceUsecase) GenerateReport(ctx context.Context, inv SkillInvocationWrite) (*ExperienceReport, error) {
	isSuccess, failureTags := uc.AnalyzeInvocation(ctx, inv)

	// Compute score.
	score := DefaultNeutralScore // default neutral
	if uc.aggregator != nil {
		if s, err := uc.ScoreSkill(ctx, inv.SkillID); err == nil {
			score = s
		}
	}

	// Build selection snapshot.
	var selectionSnapshot json.RawMessage
	if inv.SelectionReason != nil {
		selectionSnapshot, _ = json.Marshal(inv.SelectionReason)
	}

	// Rule-based flow summary.
	flowSummary := buildFlowSummary(inv, isSuccess, failureTags)

	// Rule-based optimization advice.
	optimizationAdvice := buildOptimizationAdvice(inv, isSuccess, failureTags)

	report := &ExperienceReport{
		ID:                 uuid.New().String(),
		TenantID:           "",
		SessionID:          inv.SessionID,
		InvocationID:       inv.ActivationID,
		SkillID:            inv.SkillID,
		IsSuccess:          isSuccess,
		Score:              score,
		FailureTags:        failureTags,
		FlowSummary:        flowSummary,
		OptimizationAdvice: optimizationAdvice,
		SelectionSnapshot:  selectionSnapshot,
		CreatedAt:          time.Now().UTC(),
	}

	// Persist the report.
	if uc.writer != nil {
		if err := uc.writer.Create(ctx, *report); err != nil {
			uc.lg.Warn("GenerateReport: write failed",
				loggateway.StepID("skill_intelligence.generate"),
				loggateway.Str("skill_id", inv.SkillID),
				loggateway.Err(err))
			// Non-fatal: return the report even if persistence fails.
		}
	}

	return report, nil
}

// GetExperienceReports lists experience reports for a skill.
func (uc *SkillIntelligenceUsecase) GetExperienceReports(ctx context.Context, skillID string, limit, offset int) ([]ExperienceReport, error) {
	if uc.reader == nil {
		return nil, kerrors.ServiceUnavailable("SKILL_INTELLIGENCE", "reader not available")
	}
	return uc.reader.ListBySkill(ctx, skillID, limit, offset)
}

// GetExperienceReport returns a single experience report by ID.
func (uc *SkillIntelligenceUsecase) GetExperienceReport(ctx context.Context, id string) (*ExperienceReport, error) {
	if uc.reader == nil {
		return nil, kerrors.ServiceUnavailable("SKILL_INTELLIGENCE", "reader not available")
	}
	return uc.reader.GetByID(ctx, id)
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

func buildFlowSummary(inv SkillInvocationWrite, isSuccess bool, failureTags []string) string {
	if isSuccess {
		return fmt.Sprintf("Skill %s completed successfully in %dms.", inv.SkillID, inv.DurationMS)
	}
	tagsStr := "unknown"
	if len(failureTags) > 0 {
		tagsStr = strings.Join(failureTags, ", ")
	}
	errMsg := inv.ErrorMessage
	if errMsg == "" {
		errMsg = inv.ErrorCode
	}
	if errMsg != "" {
		return fmt.Sprintf("Skill %s failed in %dms. Failure tags: [%s]. Error: %s", inv.SkillID, inv.DurationMS, tagsStr, truncateStr(errMsg, 200))
	}
	return fmt.Sprintf("Skill %s failed in %dms. Failure tags: [%s].", inv.SkillID, inv.DurationMS, tagsStr)
}

func buildOptimizationAdvice(inv SkillInvocationWrite, isSuccess bool, failureTags []string) string {
	if isSuccess {
		if inv.DurationMS > 10000 {
			return "Consider optimizing skill performance to reduce latency."
		}
		return "No optimization needed."
	}

	var advices []string
	for _, tag := range failureTags {
		switch tag {
		case FailureTagToolTimeout:
			advices = append(advices, "Add timeout handling and retry logic to the skill implementation")
		case FailureTagToolAPIError:
			advices = append(advices, "Add error handling for API failures and implement fallback behavior")
		case FailureTagParamMismatch:
			advices = append(advices, "Improve parameter validation and add clearer parameter descriptions")
		case FailureTagWrongToolChoice:
			advices = append(advices, "Refine skill description to reduce ambiguity in tool selection")
		case FailureTagContextOverflow:
			advices = append(advices, "Reduce input size or add context window management")
		case FailureTagInstructionAmbiguity:
			advices = append(advices, "Clarify skill instructions and add examples")
		default:
			advices = append(advices, "Investigate root cause and add error handling")
		}
	}
	if len(advices) == 0 {
		return "Investigate failure and add appropriate error handling."
	}
	return strings.Join(advices, "; ")
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
