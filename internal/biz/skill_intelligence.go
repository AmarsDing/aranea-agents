package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

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

// RootCauseAnalysisResult represents the result of a root cause analysis,
// decoupled from the monitor package so that biz does not import monitor.
type RootCauseAnalysisResult struct {
	RootCause  string
	FixSuggest string
	Severity   string
	Confidence float64
}

// RootCauseAnalyzer performs root cause analysis for skill failures.
// The monitor package provides the concrete implementation; biz depends on
// this interface to avoid importing monitor.
type RootCauseAnalyzer interface {
	// AnalyzeInvocationFailure analyzes a failed skill invocation and returns
	// a root cause analysis result. Returns nil if no rule matches.
	AnalyzeInvocationFailure(ctx context.Context, inv SkillInvocationWrite) (*RootCauseAnalysisResult, error)
}

// SkillIntelligenceUsecase provides skill intelligence analysis: invocation
// analysis, scoring, and experience report generation.
type SkillIntelligenceUsecase struct {
	writer           ExperienceReportWriter
	reader           ExperienceReportReader
	statsReader      ExperienceReportStatsReader
	aggregator       SkillHealthAggregator
	suggestionReader SkillEvolutionSuggestionReader
	suggestionWriter SkillEvolutionSuggestionWriter
	analyzer         RootCauseAnalyzer
	unanalyzedReader SkillInvocationUnanalyzedReader
	coordinator      *EvolutionCoordinator
	orchestrator     *SkillEvolutionOrchestrator
	lg               loggateway.Logger
	weights          ScoreWeights
}

// SkillIntelligenceConfig holds optional dependencies for SkillIntelligenceUsecase.
type SkillIntelligenceConfig struct {
	Coordinator      *EvolutionCoordinator
	Orchestrator     *SkillEvolutionOrchestrator
	UnanalyzedReader SkillInvocationUnanalyzedReader
}

// NewSkillIntelligenceUsecase constructs a SkillIntelligenceUsecase.
func NewSkillIntelligenceUsecase(
	reader ExperienceReportReader,
	writer ExperienceReportWriter,
	statsReader ExperienceReportStatsReader,
	aggregator SkillHealthAggregator,
	suggestionReader SkillEvolutionSuggestionReader,
	suggestionWriter SkillEvolutionSuggestionWriter,
	analyzer RootCauseAnalyzer,
	lg loggateway.Logger,
	opts ...SkillIntelligenceConfig,
) *SkillIntelligenceUsecase {
	uc := &SkillIntelligenceUsecase{
		writer:           writer,
		reader:           reader,
		statsReader:      statsReader,
		aggregator:       aggregator,
		suggestionReader: suggestionReader,
		suggestionWriter: suggestionWriter,
		analyzer:         analyzer,
		lg:               lg,
		weights:          DefaultScoreWeights(),
	}
	for _, opt := range opts {
		if opt.Coordinator != nil {
			uc.coordinator = opt.Coordinator
		}
		if opt.Orchestrator != nil {
			uc.orchestrator = opt.Orchestrator
		}
		if opt.UnanalyzedReader != nil {
			uc.unanalyzedReader = opt.UnanalyzedReader
		}
	}
	return uc
}

// ExperienceReportListResult holds the result of a filtered experience report query,
// including pagination total and aggregate statistics.
type ExperienceReportListResult struct {
	Reports          []ExperienceReport
	TotalCount       int
	FailureTagCounts []FailureTagCount
	RootCauseReports []ExperienceReport
}

// SkillIntelligenceRepo is the combined interface for skill intelligence data access.
type SkillIntelligenceRepo interface {
	ExperienceReportReader
	ExperienceReportWriter
	ExperienceReportStatsReader
	SkillHealthAggregator
	SkillInvocationUnanalyzedReader
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
		if data, err := json.Marshal(inv.SelectionReason); err == nil {
			selectionSnapshot = data
		} else {
			uc.lg.Warn("marshal selection reason failed", loggateway.Err(err))
		}
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
		SkillName:          inv.SkillName,
		IsSuccess:          isSuccess,
		Score:              score,
		FailureTags:        failureTags,
		FlowSummary:        flowSummary,
		OptimizationAdvice: optimizationAdvice,
		SelectionSnapshot:  selectionSnapshot,
		CreatedAt:          time.Now().UTC(),
	}

	// Root cause analysis for failed invocations.
	if !isSuccess && uc.analyzer != nil {
		if rcaResult, rcaErr := uc.analyzer.AnalyzeInvocationFailure(ctx, inv); rcaErr != nil {
			uc.lg.Warn("GenerateReport: root cause analysis failed",
				loggateway.StepID("skill_intelligence.generate"),
				loggateway.Str("skill_id", inv.SkillID),
				loggateway.Err(rcaErr))
		} else if rcaResult != nil {
			report.RootCauseAnalysis = rcaResult.RootCause
			report.SuggestedFix = rcaResult.FixSuggest
		}
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
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "reader not available")
	}
	return uc.reader.ListBySkill(ctx, skillID, limit, offset)
}

// GetExperienceReportsFiltered returns experience reports with optional skillID
// and time range filters, along with total count for pagination and aggregate
// statistics (failure tag counts and root cause reports).
// skillID empty = no skill filter; startTime/endTime nil = no time boundary.
func (uc *SkillIntelligenceUsecase) GetExperienceReportsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time, limit, offset int) (*ExperienceReportListResult, error) {
	if uc.reader == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "reader not available")
	}

	reports, totalCount, err := uc.reader.ListFiltered(ctx, skillID, startTime, endTime, limit, offset)
	if err != nil {
		uc.lg.Warn("GetExperienceReportsFiltered: ListFiltered failed",
			loggateway.StepID("skill_intelligence.list_filtered"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil, err
	}

	result := &ExperienceReportListResult{
		Reports:    reports,
		TotalCount: totalCount,
	}

	// Fetch failure tag counts if stats reader is available.
	if uc.statsReader != nil {
		tagCounts, tagErr := uc.statsReader.GetFailureTagCountsFiltered(ctx, skillID, startTime, endTime)
		if tagErr != nil {
			uc.lg.Warn("GetExperienceReportsFiltered: GetFailureTagCountsFiltered failed",
				loggateway.StepID("skill_intelligence.list_filtered"),
				loggateway.Str("skill_id", skillID),
				loggateway.Err(tagErr))
		} else {
			result.FailureTagCounts = tagCounts
		}

		rootCauseReports, rcErr := uc.statsReader.GetRootCauseReportsFiltered(ctx, skillID, startTime, endTime, 10)
		if rcErr != nil {
			uc.lg.Warn("GetExperienceReportsFiltered: GetRootCauseReportsFiltered failed",
				loggateway.StepID("skill_intelligence.list_filtered"),
				loggateway.Str("skill_id", skillID),
				loggateway.Err(rcErr))
		} else {
			result.RootCauseReports = rootCauseReports
		}
	}

	return result, nil
}

// GetExperienceReport returns a single experience report by ID.
func (uc *SkillIntelligenceUsecase) GetExperienceReport(ctx context.Context, id string) (*ExperienceReport, error) {
	if uc.reader == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "reader not available")
	}
	return uc.reader.GetByID(ctx, id)
}

// ExpirePendingSuggestions marks pending evolution suggestions as rejected
// if they have been pending for more than EvoExpirationDays (7 days).
func (uc *SkillIntelligenceUsecase) ExpirePendingSuggestions(ctx context.Context) ([]SkillEvolutionSuggestion, error) {
	if uc.suggestionReader == nil || uc.suggestionWriter == nil {
		return nil, nil
	}

	pending, err := uc.suggestionReader.ListPending(ctx, 1000, 0)
	if err != nil {
		uc.lg.Warn("ExpirePendingSuggestions: ListPending failed",
			loggateway.StepID("skill_intelligence.expire"),
			loggateway.Err(err))
		return nil, err
	}

	expirationCutoff := time.Now().UTC().Add(-EvoExpirationDays * 24 * time.Hour)
	var expired []SkillEvolutionSuggestion

	for _, sug := range pending {
		select {
		case <-ctx.Done():
			return expired, ctx.Err()
		default:
		}
		if sug.Status != EvoSuggestionPending {
			continue
		}
		if sug.CreatedAt.Before(expirationCutoff) {
			if updateErr := uc.suggestionWriter.UpdateStatus(ctx, sug.ID, EvoSuggestionRejected, "system", "auto-expired: pending for more than 7 days"); updateErr != nil {
				uc.lg.Warn("ExpirePendingSuggestions: UpdateStatus failed",
					loggateway.StepID("skill_intelligence.expire"),
					loggateway.Str("suggestion_id", sug.ID),
					loggateway.Err(updateErr))
				continue
			}
			sug.Status = EvoSuggestionRejected
			expired = append(expired, sug)
		}
	}

	if len(expired) > 0 {
		uc.lg.Info("ExpirePendingSuggestions: expired suggestions",
			loggateway.StepID("skill_intelligence.expire"),
			loggateway.Int("count", len(expired)))
	}

	return expired, nil
}

// ScanAndGenerateReports scans recent skill invocations that don't have
// experience reports yet and generates reports for them.
func (uc *SkillIntelligenceUsecase) ScanAndGenerateReports(ctx context.Context) error {
	if uc.unanalyzedReader == nil {
		uc.lg.Info("SkillIntelligenceWorker: no unanalyzed reader configured, skipping scan",
			loggateway.StepID("skill_intelligence.scan"))
		return nil
	}

	const batchSize = 100
	invs, err := uc.unanalyzedReader.ListUnanalyzed(ctx, batchSize)
	if err != nil {
		uc.lg.Warn("SkillIntelligenceWorker: ListUnanalyzed failed",
			loggateway.StepID("skill_intelligence.scan"),
			loggateway.Err(err))
		return err
	}

	if len(invs) == 0 {
		return nil
	}

	uc.lg.Info("SkillIntelligenceWorker: processing unanalyzed invocations",
		loggateway.StepID("skill_intelligence.scan"),
		loggateway.Int("count", len(invs)))

	for _, inv := range invs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// GenerateReport handles AnalyzeInvocation + ScoreSkill + RCA internally.
		if _, genErr := uc.GenerateReport(ctx, inv); genErr != nil {
			uc.lg.Warn("SkillIntelligenceWorker: GenerateReport failed for invocation",
				loggateway.StepID("skill_intelligence.scan"),
				loggateway.Str("invocation_id", inv.ActivationID),
				loggateway.Err(genErr))
			continue
		}
		// Only mark as analyzed when report generation succeeded.
		if markErr := uc.unanalyzedReader.MarkAnalyzed(ctx, inv.ActivationID); markErr != nil {
			uc.lg.Warn("SkillIntelligenceWorker: MarkAnalyzed failed",
				loggateway.StepID("skill_intelligence.scan"),
				loggateway.Str("invocation_id", inv.ActivationID),
				loggateway.Err(markErr))
		}
	}

	return nil
}

// ── Skill Evolution Suggestion methods ────────────────────────────────────────

// CheckEvolutionTriggers checks if a skill meets the conditions for generating
// an evolution suggestion. Returns a new suggestion if triggered, or nil if not.
//
// Trigger conditions (any one suffices):
//  1. 30d failure rate > 30% (existing)
//  2. 7d success rate < 60% (Curator Agent)
//  3. Same failure tag >= 5 times in 7d (Curator Agent)
//  4. Skill score < 60 (existing)
func (uc *SkillIntelligenceUsecase) CheckEvolutionTriggers(ctx context.Context, skillID string) (*SkillEvolutionSuggestion, error) {
	if uc.aggregator == nil || uc.suggestionReader == nil || uc.suggestionWriter == nil {
		return nil, nil
	}

	skillID, err := requireNonEmpty(skillID, "SKILL_INTELLIGENCE", "skill_id")
	if err != nil {
		return nil, err
	}

	// Cross-pipeline dedup: skip if another pipeline already has a pending
	// suggestion for this skill. Prefer orchestrator over legacy coordinator.
	if uc.orchestrator != nil {
		hasPending, err := uc.orchestrator.HasPendingForTarget(ctx, "skill", skillID)
		if err == nil && hasPending {
			uc.lg.Debug("CheckEvolutionTriggers: skipped, pending evolution already exists via orchestrator",
				loggateway.StepID("skill_intelligence.evo_trigger"),
				loggateway.Str("skill_id", skillID))
			return nil, nil
		}
	} else if uc.coordinator != nil && uc.coordinator.HasPendingEvolution(ctx, EvolutionTarget{Type: "skill", ID: skillID}) {
		uc.lg.Debug("CheckEvolutionTriggers: skipped, pending evolution already exists via another pipeline",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID))
		return nil, nil
	}

	// Check cooldown: if a recent suggestion exists within the cooldown period, skip.
	latest, err := uc.suggestionReader.GetLatestBySkill(ctx, skillID)
	if err != nil {
		uc.lg.Warn("CheckEvolutionTriggers: GetLatestBySkill failed",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil, err
	}
	if latest != nil {
		cooldownEnd := latest.CreatedAt.Add(EvoTriggerCooldownHours * time.Hour)
		if time.Now().UTC().Before(cooldownEnd) {
			return nil, nil
		}
	}

	// Get health metrics for the last 30 days.
	since30d := time.Now().UTC().Add(-30 * 24 * time.Hour)
	metrics, err := uc.aggregator.GetHealthMetrics(ctx, skillID, since30d)
	if err != nil {
		uc.lg.Warn("CheckEvolutionTriggers: GetHealthMetrics failed",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil, err
	}

	// Check minimum invocations for statistical significance.
	if metrics.InvocationCount < EvoTriggerMinInvocations {
		return nil, nil
	}

	// Determine trigger conditions. Collect all matching types to avoid
	// losing semantic information when multiple conditions fire.
	var triggerTypes []EvolutionSuggestionType
	var triggerReasons []string

	// Condition 1: 30d failure rate > threshold.
	failureRate := 1.0 - metrics.SuccessRate
	if failureRate > EvoTriggerFailureRate {
		triggerTypes = append(triggerTypes, EvoSuggestionFixFailure)
		triggerReasons = append(triggerReasons, fmt.Sprintf("30d failure rate %.1f%% exceeds threshold %.1f%% (%d invocations)",
			failureRate*100, EvoTriggerFailureRate*100, metrics.InvocationCount))
	}

	// Condition 2: 7d success rate < 60%.
	since7d := time.Now().UTC().Add(-7 * 24 * time.Hour)
	metrics7d, err7d := uc.aggregator.GetHealthMetrics(ctx, skillID, since7d)
	if err7d != nil {
		uc.lg.Warn("CheckEvolutionTriggers: GetHealthMetrics(7d) failed",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err7d))
	} else if metrics7d.InvocationCount >= EvoTrigger7dMinInvocations && metrics7d.SuccessRate < EvoTrigger7dSuccessRate {
		triggerTypes = append(triggerTypes, EvoSuggestionFixFailure)
		triggerReasons = append(triggerReasons, fmt.Sprintf("7d success rate %.1f%% below threshold %.1f%% (%d invocations)",
			metrics7d.SuccessRate*100, EvoTrigger7dSuccessRate*100, metrics7d.InvocationCount))
	}

	// Condition 3: Same failure tag >= 5 times in 7d.
	tagCounts, tagErr := uc.aggregator.GetFailureTagCounts(ctx, skillID, since7d)
	if tagErr != nil {
		uc.lg.Warn("CheckEvolutionTriggers: GetFailureTagCounts failed",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(tagErr))
	} else {
		for _, tc := range tagCounts {
			if tc.Count >= EvoTriggerSameTagThreshold {
				triggerTypes = append(triggerTypes, EvoSuggestionFixFailure)
				triggerReasons = append(triggerReasons, fmt.Sprintf("failure tag %q appears %d times in 7d (threshold %d)",
					tc.Tag, tc.Count, EvoTriggerSameTagThreshold))
				break // one matching tag is enough
			}
		}
	}

	// Condition 4: Skill score < threshold.
	score, scoreErr := uc.ScoreSkill(ctx, skillID)
	if scoreErr == nil && score < EvoTriggerScoreThreshold {
		triggerTypes = append(triggerTypes, EvoSuggestionBoostEfficiency)
		triggerReasons = append(triggerReasons, fmt.Sprintf("Skill score %d below threshold %d", score, EvoTriggerScoreThreshold))
	}

	if len(triggerTypes) == 0 {
		return nil, nil
	}

	// Pick the primary type (first triggered) and join all reasons.
	triggerType := triggerTypes[0]
	triggerReason := strings.Join(triggerReasons, "; ")

	suggestion := &SkillEvolutionSuggestion{
		ID:            uuid.New().String(),
		SkillID:       skillID,
		Type:          triggerType,
		Status:        EvoSuggestionPending,
		TriggerReason: triggerReason,
		LifecycleStatus: EvoLifecycleDraft,
		CreatedAt:     time.Now().UTC(),
	}

	if err := uc.suggestionWriter.Create(ctx, *suggestion); err != nil {
		uc.lg.Warn("CheckEvolutionTriggers: Create suggestion failed",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil, err
	}

	return suggestion, nil
}

// RunCuratorFlow executes the full Curator Agent semi-automatic evolution
// pipeline for a skill: trigger detection → draft generation → sandbox
// verification → lifecycle update.
//
// The flow is:
//  1. CheckEvolutionTriggers — detect if the skill needs evolution
//  2. GenerateDraftBody — produce a rule-based (v1) draft of the improved skill
//  3. UpdateLifecycleStatus — mark as "validating"
//  4. RuleBasedSandboxValidation — validate the draft in sandbox
//  5. UpdateLifecycleStatus — mark as "ready" if passed, "draft" if failed
//
// Returns the suggestion if triggered, or nil if no trigger condition was met.
func (uc *SkillIntelligenceUsecase) RunCuratorFlow(ctx context.Context, skillID string) (*SkillEvolutionSuggestion, error) {
	// Step 1: Trigger detection.
	suggestion, err := uc.CheckEvolutionTriggers(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if suggestion == nil {
		return nil, nil
	}

	// Step 2: Generate draft body (rule-based for v1).
	draft := uc.generateRuleBasedDraft(suggestion)
	if err := uc.suggestionWriter.UpdateDraftBody(ctx, suggestion.ID, draft); err != nil {
		uc.lg.Warn("RunCuratorFlow: UpdateDraftBody failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(err))
		return nil, apierror.Internal("SKILL_INTELLIGENCE", "failed to persist draft body for suggestion %s: %s", suggestion.ID, err.Error())
	}
	suggestion.DraftSkillBody = draft

	// Step 3: Set lifecycle to validating.
	if lcErr := uc.suggestionWriter.UpdateLifecycleStatus(ctx, suggestion.ID, EvoLifecycleValidating); lcErr != nil {
		uc.lg.Warn("RunCuratorFlow: UpdateLifecycleStatus(validating) failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(lcErr))
	}
	suggestion.LifecycleStatus = EvoLifecycleValidating

	// Step 4: Rule-based sandbox validation.
	passed := uc.ruleBasedSandboxValidation(suggestion)
	resultJSON, _ := json.Marshal(map[string]any{
		"passed":  passed,
		"checks":  []map[string]any{
			{"name": "draft_body_not_empty", "passed": suggestion.DraftSkillBody != ""},
			{"name": "draft_body_length", "passed": len(suggestion.DraftSkillBody) < 10000},
			{"name": "skill_id_valid", "passed": suggestion.SkillID != ""},
		},
		"message": func() string {
			if passed {
				return "All validation checks passed"
			}
			return "Some validation checks failed"
		}(),
	})
	if sbErr := uc.suggestionWriter.UpdateSandboxResult(ctx, suggestion.ID, passed, resultJSON); sbErr != nil {
		uc.lg.Warn("RunCuratorFlow: UpdateSandboxResult failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(sbErr))
	}
	suggestion.SandboxPassed = passed
	suggestion.SandboxResult = resultJSON

	// Step 5: Update lifecycle based on validation result.
	// Rule-based template drafts are always marked as "draft" (needs human
	// editing) rather than "ready", because they contain generic placeholder
	// suggestions instead of skill-specific analysis. Only LLM-generated
	// drafts should be promoted to "ready" after passing validation.
	lifecycleStatus := EvoLifecycleDraft
	if lcErr := uc.suggestionWriter.UpdateLifecycleStatus(ctx, suggestion.ID, lifecycleStatus); lcErr != nil {
		uc.lg.Warn("RunCuratorFlow: UpdateLifecycleStatus(final) failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(lcErr))
	}
	suggestion.LifecycleStatus = lifecycleStatus

	return suggestion, nil
}

// generateRuleBasedDraft produces a rule-based draft skill body for the given
// suggestion type. For v1, this is the primary draft generator; LLM integration
// will be added in a future iteration.
func (uc *SkillIntelligenceUsecase) generateRuleBasedDraft(suggestion *SkillEvolutionSuggestion) string {
	switch suggestion.Type {
	case EvoSuggestionFixFailure:
		return fmt.Sprintf("# Skill Evolution Draft (fix_failure)\n\n"+
			"Original skill: %s\n"+
			"Trigger reason: %s\n\n"+
			"## Suggested improvements:\n"+
			"1. Add error handling for common failure patterns\n"+
			"2. Improve parameter validation\n"+
			"3. Add retry logic for transient failures\n",
			suggestion.SkillID, suggestion.TriggerReason)
	case EvoSuggestionBoostEfficiency:
		return fmt.Sprintf("# Skill Evolution Draft (boost_efficiency)\n\n"+
			"Original skill: %s\n"+
			"Trigger reason: %s\n\n"+
			"## Suggested improvements:\n"+
			"1. Optimize prompt to reduce token usage\n"+
			"2. Cache frequently used results\n"+
			"3. Reduce unnecessary tool calls\n",
			suggestion.SkillID, suggestion.TriggerReason)
	case EvoSuggestionMergeDuplicate:
		return fmt.Sprintf("# Skill Evolution Draft (merge_duplicate)\n\n"+
			"Original skill: %s\n"+
			"Trigger reason: %s\n\n"+
			"## Suggested improvements:\n"+
			"1. Consolidate overlapping functionality\n"+
			"2. Unify parameter interfaces\n"+
			"3. Merge description and instructions\n",
			suggestion.SkillID, suggestion.TriggerReason)
	case EvoSuggestionCreateSkill:
		return fmt.Sprintf("# Skill Evolution Draft (create_skill)\n\n"+
			"Trigger reason: %s\n\n"+
			"## Suggested actions:\n"+
			"1. Define skill scope and purpose\n"+
			"2. Specify tool dependencies and parameters\n"+
			"3. Write skill instructions and examples\n",
			suggestion.TriggerReason)
	default:
		return fmt.Sprintf("# Skill Evolution Draft\n\nOriginal skill: %s\nTrigger: %s\n",
			suggestion.SkillID, suggestion.TriggerReason)
	}
}

// ruleBasedSandboxValidation performs basic rule-based validation on the
// suggestion draft. Returns true if all checks pass.
func (uc *SkillIntelligenceUsecase) ruleBasedSandboxValidation(suggestion *SkillEvolutionSuggestion) bool {
	if suggestion.DraftSkillBody == "" {
		return false
	}
	if len(suggestion.DraftSkillBody) >= 10000 {
		return false
	}
	if suggestion.SkillID == "" {
		return false
	}
	return true
}

// GenerateDraftForSuggestion generates a draft skill body for an existing
// evolution suggestion, updates the suggestion, and returns the draft.
func (uc *SkillIntelligenceUsecase) GenerateDraftForSuggestion(ctx context.Context, suggestionID string) (string, error) {
	suggestion, err := uc.GetEvolutionSuggestion(ctx, suggestionID)
	if err != nil {
		return "", err
	}
	if suggestion == nil {
		return "", apierror.NotFound("SKILL_INTELLIGENCE", "suggestion not found: %s", suggestionID)
	}

	draft := uc.generateRuleBasedDraft(suggestion)

	if err := uc.UpdateSuggestionDraftBody(ctx, suggestionID, draft); err != nil {
		uc.lg.Warn("GenerateDraftForSuggestion: UpdateSuggestionDraftBody failed",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
		return "", err
	}

	if lcErr := uc.UpdateSuggestionLifecycleStatus(ctx, suggestionID, EvoLifecycleDraft); lcErr != nil {
		uc.lg.Warn("GenerateDraftForSuggestion: UpdateSuggestionLifecycleStatus failed",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(lcErr))
	}

	return draft, nil
}

// CreateSuggestion delegates to the suggestion writer.
func (uc *SkillIntelligenceUsecase) CreateSuggestion(ctx context.Context, suggestion SkillEvolutionSuggestion) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.suggestionWriter.Create(ctx, suggestion)
}

// GetEvolutionSuggestion returns a single evolution suggestion by ID.
func (uc *SkillIntelligenceUsecase) GetEvolutionSuggestion(ctx context.Context, id string) (*SkillEvolutionSuggestion, error) {
	if uc.suggestionReader == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	return uc.suggestionReader.GetByID(ctx, id)
}

// UpdateSuggestionDraftBody updates the draft skill body of an evolution suggestion.
func (uc *SkillIntelligenceUsecase) UpdateSuggestionDraftBody(ctx context.Context, id string, draftBody string) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.suggestionWriter.UpdateDraftBody(ctx, id, draftBody)
}

// UpdateSuggestionSandboxResult updates the sandbox validation result of an evolution suggestion.
func (uc *SkillIntelligenceUsecase) UpdateSuggestionSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.suggestionWriter.UpdateSandboxResult(ctx, id, passed, result)
}

// UpdateSuggestionLifecycleStatus updates the lifecycle status of an evolution suggestion.
func (uc *SkillIntelligenceUsecase) UpdateSuggestionLifecycleStatus(ctx context.Context, id string, lifecycleStatus EvolutionLifecycleStatus) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.suggestionWriter.UpdateLifecycleStatus(ctx, id, lifecycleStatus)
}

// ListEvolutionSuggestions lists evolution suggestions for a skill, optionally filtered by status.
func (uc *SkillIntelligenceUsecase) ListEvolutionSuggestions(ctx context.Context, skillID string, status EvolutionSuggestionStatus, limit, offset int) ([]SkillEvolutionSuggestion, error) {
	if uc.suggestionReader == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	return uc.suggestionReader.ListBySkill(ctx, skillID, status, limit, offset)
}

// CountEvolutionSuggestions returns the total count of evolution suggestions for a skill, optionally filtered by status.
func (uc *SkillIntelligenceUsecase) CountEvolutionSuggestions(ctx context.Context, skillID string, status EvolutionSuggestionStatus) (int, error) {
	if uc.suggestionReader == nil {
		return 0, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	return uc.suggestionReader.CountBySkill(ctx, skillID, status)
}

// ApproveSuggestion approves a pending evolution suggestion.
func (uc *SkillIntelligenceUsecase) ApproveSuggestion(ctx context.Context, id, approvedBy string) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.suggestionWriter.UpdateStatus(ctx, id, EvoSuggestionApproved, approvedBy, "")
}

// RejectSuggestion rejects a pending evolution suggestion.
func (uc *SkillIntelligenceUsecase) RejectSuggestion(ctx context.Context, id, rejectedBy, reason string) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.suggestionWriter.UpdateStatus(ctx, id, EvoSuggestionRejected, rejectedBy, reason)
}

// ── Bridge: SkillEvolutionSuggestion → SkillProposal ─────────────────────────
//
// Deprecated: Transitional bridge function. Will be removed once SkillProposal
// is fully deprecated. Use UnifiedEvolutionSuggestion directly instead.

// ProposalFromSuggestion converts a SkillEvolutionSuggestion into a
// SkillProposal for interoperability with the SkillEvolutionUsecase pipeline.
// Fields that have no direct equivalent are left at their zero values.
//
// Deprecated: Use UnifiedEvolutionSuggestion directly. Construct with
// UnifiedEvolutionSuggestion{TargetType: "skill", ActionType: "improve_skill", TargetID: s.SkillID, DraftBody: s.DraftSkillBody}.
func (uc *SkillIntelligenceUsecase) ProposalFromSuggestion(s SkillEvolutionSuggestion) SkillProposal {
	return SkillProposal{
		ID:         s.ID,
		AgentID:    "",                       // no equivalent; SkillEvolutionSuggestion is skill-scoped
		SkillName:  "",                       // no direct equivalent; caller should populate from skill lookup
		SkillMD:    s.DraftSkillBody,
		Status:     SuggestionStatusToProposal(s.Status),
		ApprovedBy: s.ApprovedBy,
		RejectedBy: s.RejectedBy,
		CreatedAt:  s.CreatedAt,
		ApprovedAt: s.ResolvedAt,
	}
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
// Heuristic rules:
//   - High success rate (>0.9) → bonus 0.2
//   - Low avg duration (<5s) → bonus 0.1
//   - Low token usage (<1000) → bonus 0.1
//   - Clamped to [0, 1]
func computeHeuristicFeedbackScore(m *SkillHealthMetrics) float64 {
	if m == nil || m.InvocationCount == 0 {
		return 0
	}

	var score float64

	// Base: proportional to success rate (0-0.6 range).
	score = m.SuccessRate * 0.6

	// Bonus: high success rate.
	if m.SuccessRate > 0.9 {
		score += 0.2
	}

	// Bonus: low latency.
	if m.AvgDurationMS > 0 && m.AvgDurationMS < 5000 {
		score += 0.1
	}

	// Bonus: low token usage.
	if m.AvgTokenUsage > 0 && m.AvgTokenUsage < 1000 {
		score += 0.1
	}

	if score > 1 {
		score = 1
	}
	if score < 0 {
		score = 0
	}
	return score
}
