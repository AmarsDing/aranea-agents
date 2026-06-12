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
// Scoring and report generation are delegated to SkillScoringUsecase and
// SkillReportUsecase respectively (AS-COG-01 decomposition).
type SkillIntelligenceUsecase struct {
	scorer           *SkillScoringUsecase
	reporter         *SkillReportUsecase
	suggestionReader SkillEvolutionSuggestionReader
	suggestionWriter SkillEvolutionSuggestionWriter
	unifiedReader    UnifiedEvolutionReader
	unifiedWriter    UnifiedEvolutionWriter
	aggregator       SkillHealthAggregator
	unanalyzedReader SkillInvocationUnanalyzedReader
	coordinator      *EvolutionCoordinator
	orchestrator     *SkillEvolutionOrchestrator
	gate             SkillGateVerifier
	lg               loggateway.Logger
}

// SkillIntelligenceConfig holds optional dependencies for SkillIntelligenceUsecase.
type SkillIntelligenceConfig struct {
	Scorer           *SkillScoringUsecase
	Reporter         *SkillReportUsecase
	Coordinator      *EvolutionCoordinator
	Orchestrator     *SkillEvolutionOrchestrator
	UnanalyzedReader SkillInvocationUnanalyzedReader
	Gate             SkillGateVerifier
	UnifiedReader    UnifiedEvolutionReader
	UnifiedWriter    UnifiedEvolutionWriter
}

// NewSkillIntelligenceUsecase constructs a SkillIntelligenceUsecase.
func NewSkillIntelligenceUsecase(
	scorer *SkillScoringUsecase,
	reporter *SkillReportUsecase,
	suggestionReader SkillEvolutionSuggestionReader,
	suggestionWriter SkillEvolutionSuggestionWriter,
	unifiedReader UnifiedEvolutionReader,
	unifiedWriter UnifiedEvolutionWriter,
	aggregator SkillHealthAggregator,
	analyzer RootCauseAnalyzer,
	lg loggateway.Logger,
	opts ...SkillIntelligenceConfig,
) *SkillIntelligenceUsecase {
	uc := &SkillIntelligenceUsecase{
		scorer:           scorer,
		reporter:         reporter,
		suggestionReader: suggestionReader,
		suggestionWriter: suggestionWriter,
		unifiedReader:    unifiedReader,
		unifiedWriter:    unifiedWriter,
		aggregator:       aggregator,
		lg:               lg,
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
		if opt.Gate != nil {
			uc.gate = opt.Gate
		}
	}
	_ = analyzer // analyzer is now used by reporter, not directly by this usecase
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

// ── Delegated methods (scorer) ────────────────────────────────────────────────

// AnalyzeInvocation delegates to SkillScoringUsecase.
func (uc *SkillIntelligenceUsecase) AnalyzeInvocation(ctx context.Context, inv SkillInvocationWrite) (isSuccess bool, failureTags []string) {
	return uc.scorer.AnalyzeInvocation(ctx, inv)
}

// ScoreSkill delegates to SkillScoringUsecase.
func (uc *SkillIntelligenceUsecase) ScoreSkill(ctx context.Context, skillID string) (int, error) {
	return uc.scorer.ScoreSkill(ctx, skillID)
}

// ── Delegated methods (reporter) ──────────────────────────────────────────────

// GenerateReport delegates to SkillReportUsecase.
func (uc *SkillIntelligenceUsecase) GenerateReport(ctx context.Context, inv SkillInvocationWrite) (*ExperienceReport, error) {
	return uc.reporter.GenerateReport(ctx, inv)
}

// GetExperienceReports delegates to SkillReportUsecase.
func (uc *SkillIntelligenceUsecase) GetExperienceReports(ctx context.Context, skillID string, limit, offset int) ([]ExperienceReport, error) {
	return uc.reporter.GetExperienceReports(ctx, skillID, limit, offset)
}

// GetExperienceReportsFiltered delegates to SkillReportUsecase.
func (uc *SkillIntelligenceUsecase) GetExperienceReportsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time, limit, offset int) (*ExperienceReportListResult, error) {
	return uc.reporter.GetExperienceReportsFiltered(ctx, skillID, startTime, endTime, limit, offset)
}

// GetExperienceReport delegates to SkillReportUsecase.
func (uc *SkillIntelligenceUsecase) GetExperienceReport(ctx context.Context, id string) (*ExperienceReport, error) {
	return uc.reporter.GetExperienceReport(ctx, id)
}

// ScanAndGenerateReports delegates to SkillReportUsecase.
func (uc *SkillIntelligenceUsecase) ScanAndGenerateReports(ctx context.Context) error {
	return uc.reporter.ScanAndGenerateReports(ctx)
}

// ── Skill Evolution Suggestion methods ────────────────────────────────────────

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
	// Prefer unified reader over legacy suggestion reader.
	if uc.unifiedReader != nil {
		latestUnified, uErr := uc.unifiedReader.GetLatestByTarget(ctx, "skill", skillID)
		if uErr == nil && latestUnified != nil {
			cooldownEnd := latestUnified.CreatedAt.Add(EvoTriggerCooldownHours * time.Hour)
			if time.Now().UTC().Before(cooldownEnd) {
				return nil, nil
			}
		}
	} else {
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
	score, scoreErr := uc.scorer.ScoreSkill(ctx, skillID)
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
		ID:              uuid.New().String(),
		SkillID:         skillID,
		Type:            triggerType,
		Status:          EvoSuggestionPending,
		TriggerReason:   triggerReason,
		LifecycleStatus: EvoLifecycleDraft,
		CreatedAt:       time.Now().UTC(),
	}

	// Write to unified store first (if available), then bridge to legacy store.
	if uc.unifiedWriter != nil {
		unified := UnifiedEvolutionSuggestion{
			ID:              suggestion.ID,
			TargetType:      EvolutionTargetSkill,
			TargetID:        skillID,
			ActionType:      legacyTriggerToActionType(triggerType),
			TriggerSource:   "health",
			TriggerReason:   triggerReason,
			Status:          "pending",
			Priority:        1,
			LifecycleStatus: "draft",
			CreatedAt:       suggestion.CreatedAt,
		}
		if uErr := uc.unifiedWriter.Create(ctx, unified); uErr != nil {
			uc.lg.Warn("CheckEvolutionTriggers: unified Create failed, falling back to legacy",
				loggateway.StepID("skill_intelligence.evo_trigger"),
				loggateway.Str("skill_id", skillID),
				loggateway.Err(uErr))
			// Fallback: write to legacy store.
			if err := uc.suggestionWriter.Create(ctx, *suggestion); err != nil {
				uc.lg.Warn("CheckEvolutionTriggers: legacy Create suggestion failed",
					loggateway.StepID("skill_intelligence.evo_trigger"),
					loggateway.Str("skill_id", skillID),
					loggateway.Err(err))
				return nil, err
			}
		} else {
			// Bridge: also write to legacy store for backward compatibility.
			if bErr := uc.suggestionWriter.Create(ctx, *suggestion); bErr != nil {
				uc.lg.Debug("CheckEvolutionTriggers: bridge write to legacy store failed (non-fatal)",
					loggateway.StepID("skill_intelligence.evo_trigger"),
					loggateway.Err(bErr))
			}
		}
	} else {
		// No unified writer: use legacy path.
		if err := uc.suggestionWriter.Create(ctx, *suggestion); err != nil {
			uc.lg.Warn("CheckEvolutionTriggers: Create suggestion failed",
				loggateway.StepID("skill_intelligence.evo_trigger"),
				loggateway.Str("skill_id", skillID),
				loggateway.Err(err))
			return nil, err
		}
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

	// Write draft body: prefer unified writer, fallback to legacy.
	if uc.unifiedWriter != nil {
		if uErr := uc.unifiedWriter.UpdateDraftBody(ctx, suggestion.ID, draft); uErr != nil {
			uc.lg.Warn("RunCuratorFlow: unified UpdateDraftBody failed, falling back to legacy",
				loggateway.StepID("skill_intelligence.curator_flow"),
				loggateway.Str("suggestion_id", suggestion.ID),
				loggateway.Err(uErr))
			if err := uc.suggestionWriter.UpdateDraftBody(ctx, suggestion.ID, draft); err != nil {
				uc.lg.Warn("RunCuratorFlow: legacy UpdateDraftBody failed",
					loggateway.StepID("skill_intelligence.curator_flow"),
					loggateway.Str("suggestion_id", suggestion.ID),
					loggateway.Err(err))
				return nil, apierror.Internal("SKILL_INTELLIGENCE", "failed to persist draft body for suggestion %s: %s", suggestion.ID, err.Error())
			}
		} else {
			// Bridge: also update legacy store.
			_ = uc.suggestionWriter.UpdateDraftBody(ctx, suggestion.ID, draft)
		}
	} else {
		if err := uc.suggestionWriter.UpdateDraftBody(ctx, suggestion.ID, draft); err != nil {
			uc.lg.Warn("RunCuratorFlow: UpdateDraftBody failed",
				loggateway.StepID("skill_intelligence.curator_flow"),
				loggateway.Str("suggestion_id", suggestion.ID),
				loggateway.Err(err))
			return nil, apierror.Internal("SKILL_INTELLIGENCE", "failed to persist draft body for suggestion %s: %s", suggestion.ID, err.Error())
		}
	}
	suggestion.DraftSkillBody = draft

	// Step 3: Set lifecycle to validating.
	if uc.unifiedWriter != nil {
		_ = uc.unifiedWriter.UpdateLifecycleStatus(ctx, suggestion.ID, "validating")
		// Bridge.
		_ = uc.suggestionWriter.UpdateLifecycleStatus(ctx, suggestion.ID, EvoLifecycleValidating)
	} else {
		if lcErr := uc.suggestionWriter.UpdateLifecycleStatus(ctx, suggestion.ID, EvoLifecycleValidating); lcErr != nil {
			uc.lg.Warn("RunCuratorFlow: UpdateLifecycleStatus(validating) failed",
				loggateway.StepID("skill_intelligence.curator_flow"),
				loggateway.Str("suggestion_id", suggestion.ID),
				loggateway.Err(lcErr))
		}
	}
	suggestion.LifecycleStatus = EvoLifecycleValidating

	// Step 4: Sandbox validation — use GateVerifier if available, otherwise rule-based fallback.
	var passed bool
	var resultJSON json.RawMessage
	if uc.gate != nil {
		gateResult, gateErr := uc.gate.Verify(ctx, suggestion.SkillID, suggestion.DraftSkillBody, nil)
		if gateErr != nil {
			uc.lg.Warn("RunCuratorFlow: GateVerifier failed",
				loggateway.StepID("skill_intelligence.curator_flow"),
				loggateway.Str("suggestion_id", suggestion.ID),
				loggateway.Err(gateErr))
			passed = false
		} else {
			passed = gateResult.Passed
		}
		// Build sandbox result from actual gate checks.
		var checkResults []map[string]any
		if gateResult != nil {
			for _, c := range gateResult.Checks {
				checkResults = append(checkResults, map[string]any{
					"name": c.Name, "passed": c.Passed, "reason": c.Reason,
				})
			}
		}
		resultJSON, _ = json.Marshal(map[string]any{
			"passed":  passed,
			"checks":  checkResults,
			"message": func() string {
				if passed {
					return "All validation checks passed"
				}
				return "Some validation checks failed"
			}(),
		})
	} else {
		passed = uc.ruleBasedSandboxValidation(suggestion)
		resultJSON, _ = json.Marshal(map[string]any{
			"passed": passed,
			"checks": []map[string]any{
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
	}
	if uc.unifiedWriter != nil {
		_ = uc.unifiedWriter.UpdateSandboxResult(ctx, suggestion.ID, passed, resultJSON)
		// Bridge.
		_ = uc.suggestionWriter.UpdateSandboxResult(ctx, suggestion.ID, passed, resultJSON)
	} else {
		if sbErr := uc.suggestionWriter.UpdateSandboxResult(ctx, suggestion.ID, passed, resultJSON); sbErr != nil {
			uc.lg.Warn("RunCuratorFlow: UpdateSandboxResult failed",
				loggateway.StepID("skill_intelligence.curator_flow"),
				loggateway.Str("suggestion_id", suggestion.ID),
				loggateway.Err(sbErr))
		}
	}
	suggestion.SandboxPassed = passed
	suggestion.SandboxResult = resultJSON

	// Step 5: Update lifecycle based on validation result.
	// Rule-based template drafts are always marked as "draft" (needs human
	// editing) rather than "ready", because they contain generic placeholder
	// suggestions instead of skill-specific analysis. Only LLM-generated
	// drafts should be promoted to "ready" after passing validation.
	lifecycleStatus := EvoLifecycleDraft
	if uc.unifiedWriter != nil {
		_ = uc.unifiedWriter.UpdateLifecycleStatus(ctx, suggestion.ID, "draft")
		// Bridge.
		_ = uc.suggestionWriter.UpdateLifecycleStatus(ctx, suggestion.ID, lifecycleStatus)
	} else {
		if lcErr := uc.suggestionWriter.UpdateLifecycleStatus(ctx, suggestion.ID, lifecycleStatus); lcErr != nil {
			uc.lg.Warn("RunCuratorFlow: UpdateLifecycleStatus(final) failed",
				loggateway.StepID("skill_intelligence.curator_flow"),
				loggateway.Str("suggestion_id", suggestion.ID),
				loggateway.Err(lcErr))
		}
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
			"Trigger reason: %s\n"+
			"Source reports: %d experience report(s) contributed to this trigger\n\n"+
			"## Analysis\n"+
			"This skill has been flagged due to repeated failures observed in experience reports.\n"+
			"Source report data indicates actionable failure patterns that need correction.\n\n"+
			"## Suggested improvements:\n"+
			"1. Add error handling for common failure patterns identified in reports\n"+
			"2. Improve parameter validation based on observed error codes\n"+
			"3. Add retry logic for transient failures\n"+
			"4. Review and update failure tag taxonomy for better classification\n",
			suggestion.SkillID, suggestion.TriggerReason, len(suggestion.SourceReportIDs))
	case EvoSuggestionBoostEfficiency:
		return fmt.Sprintf("# Skill Evolution Draft (boost_efficiency)\n\n"+
			"Original skill: %s\n"+
			"Parent version: %s\n"+
			"Trigger reason: %s\n\n"+
			"## Analysis\n"+
			"This skill's efficiency score is below the acceptable threshold.\n"+
			"The parent version (%s) may contain suboptimal patterns that can be refined.\n\n"+
			"## Suggested improvements:\n"+
			"1. Optimize prompt to reduce token usage\n"+
			"2. Cache frequently used results\n"+
			"3. Reduce unnecessary tool calls\n"+
			"4. Streamline instruction flow to lower average duration\n",
			suggestion.SkillID, suggestion.ParentVersionID, suggestion.TriggerReason, suggestion.ParentVersionID)
	case EvoSuggestionMergeDuplicate:
		return fmt.Sprintf("# Skill Evolution Draft (merge_duplicate)\n\n"+
			"Original skill: %s\n"+
			"Trigger reason: %s\n\n"+
			"## Dedup Analysis\n"+
			"Similarity analysis has identified this skill as a candidate for merging with\n"+
			"another skill that shares significant overlap in name, description, or body content.\n"+
			"Jaccard similarity across multiple dimensions exceeded the dedup threshold.\n\n"+
			"## Suggested improvements:\n"+
			"1. Consolidate overlapping functionality into a unified skill\n"+
			"2. Unify parameter interfaces across duplicate entries\n"+
			"3. Merge description and instructions, preserving unique aspects\n"+
			"4. Add redirect markers in deprecated skill for backward compatibility\n",
			suggestion.SkillID, suggestion.TriggerReason)
	case EvoSuggestionCreateSkill:
		return fmt.Sprintf("# Skill Evolution Draft (create_skill)\n\n"+
			"Trigger reason: %s\n\n"+
			"## Pattern Analysis\n"+
			"A recurring tool-call pattern has been detected that suggests the need for a new skill.\n"+
			"The pattern description and historical invocation data indicate a reusable workflow\n"+
			"that is not yet captured by any existing skill.\n\n"+
			"## Suggested actions:\n"+
			"1. Define skill scope and purpose based on the observed pattern\n"+
			"2. Specify tool dependencies and parameters from invocation history\n"+
			"3. Write skill instructions and examples reflecting the detected workflow\n"+
			"4. Validate the new skill against existing skills to avoid duplication\n",
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
// Also bridges to the unified writer if available.
func (uc *SkillIntelligenceUsecase) CreateSuggestion(ctx context.Context, suggestion SkillEvolutionSuggestion) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	if err := uc.suggestionWriter.Create(ctx, suggestion); err != nil {
		return err
	}
	// Bridge to unified store.
	if uc.unifiedWriter != nil {
		unified := unifiedToLegacySuggestion(&suggestion)
		if bErr := uc.unifiedWriter.Create(ctx, unified); bErr != nil {
			uc.lg.Debug("CreateSuggestion: bridge write to unified store failed (non-fatal)",
				loggateway.Err(bErr))
		}
	}
	return nil
}

// GetEvolutionSuggestion returns a single evolution suggestion by ID.
// Prefers unified reader; falls back to legacy suggestion reader.
func (uc *SkillIntelligenceUsecase) GetEvolutionSuggestion(ctx context.Context, id string) (*SkillEvolutionSuggestion, error) {
	if uc.unifiedReader != nil {
		unified, err := uc.unifiedReader.GetByID(ctx, id)
		if err == nil && unified != nil {
			return unifiedToLegacySuggestionPtr(unified), nil
		}
		// Fall through to legacy on error.
	}
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
	if err := uc.suggestionWriter.UpdateDraftBody(ctx, id, draftBody); err != nil {
		return err
	}
	if uc.unifiedWriter != nil {
		_ = uc.unifiedWriter.UpdateDraftBody(ctx, id, draftBody)
	}
	return nil
}

// UpdateSuggestionSandboxResult updates the sandbox validation result of an evolution suggestion.
func (uc *SkillIntelligenceUsecase) UpdateSuggestionSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	if err := uc.suggestionWriter.UpdateSandboxResult(ctx, id, passed, result); err != nil {
		return err
	}
	if uc.unifiedWriter != nil {
		_ = uc.unifiedWriter.UpdateSandboxResult(ctx, id, passed, result)
	}
	return nil
}

// UpdateSuggestionLifecycleStatus updates the lifecycle status of an evolution suggestion.
func (uc *SkillIntelligenceUsecase) UpdateSuggestionLifecycleStatus(ctx context.Context, id string, lifecycleStatus EvolutionLifecycleStatus) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	if err := uc.suggestionWriter.UpdateLifecycleStatus(ctx, id, lifecycleStatus); err != nil {
		return err
	}
	if uc.unifiedWriter != nil {
		_ = uc.unifiedWriter.UpdateLifecycleStatus(ctx, id, string(lifecycleStatus))
	}
	return nil
}

// ListEvolutionSuggestions lists evolution suggestions for a skill, optionally filtered by status.
// Prefers unified reader; falls back to legacy suggestion reader.
func (uc *SkillIntelligenceUsecase) ListEvolutionSuggestions(ctx context.Context, skillID string, status EvolutionSuggestionStatus, limit, offset int) ([]SkillEvolutionSuggestion, error) {
	if uc.unifiedReader != nil {
		unifiedList, err := uc.unifiedReader.ListByTarget(ctx, "skill", skillID, string(status), limit, offset)
		if err == nil {
			result := make([]SkillEvolutionSuggestion, len(unifiedList))
			for i, u := range unifiedList {
				result[i] = *unifiedToLegacySuggestionPtr(&u)
			}
			return result, nil
		}
		// Fall through to legacy on error.
	}
	if uc.suggestionReader == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	return uc.suggestionReader.ListBySkill(ctx, skillID, status, limit, offset)
}

// CountEvolutionSuggestions returns the total count of evolution suggestions for a skill, optionally filtered by status.
// Prefers unified reader; falls back to legacy suggestion reader.
func (uc *SkillIntelligenceUsecase) CountEvolutionSuggestions(ctx context.Context, skillID string, status EvolutionSuggestionStatus) (int, error) {
	if uc.unifiedReader != nil {
		count, err := uc.unifiedReader.CountByTarget(ctx, "skill", skillID, string(status))
		if err == nil {
			return count, nil
		}
		// Fall through to legacy on error.
	}
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
	if err := uc.suggestionWriter.UpdateStatus(ctx, id, EvoSuggestionApproved, approvedBy, ""); err != nil {
		return err
	}
	if uc.unifiedWriter != nil {
		_ = uc.unifiedWriter.UpdateStatus(ctx, id, "approved", approvedBy, "")
	}
	return nil
}

// RejectSuggestion rejects a pending evolution suggestion.
func (uc *SkillIntelligenceUsecase) RejectSuggestion(ctx context.Context, id, rejectedBy, reason string) error {
	if uc.suggestionWriter == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	if err := uc.suggestionWriter.UpdateStatus(ctx, id, EvoSuggestionRejected, rejectedBy, reason); err != nil {
		return err
	}
	if uc.unifiedWriter != nil {
		_ = uc.unifiedWriter.UpdateStatus(ctx, id, "rejected", rejectedBy, reason)
	}
	return nil
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

// ── Bridge functions: Unified ↔ Legacy ────────────────────────────────────────

// unifiedToLegacySuggestion converts a SkillEvolutionSuggestion to a UnifiedEvolutionSuggestion
// for bridging writes to the unified store.
func unifiedToLegacySuggestion(s *SkillEvolutionSuggestion) UnifiedEvolutionSuggestion {
	return UnifiedEvolutionSuggestion{
		ID:              s.ID,
		TargetType:      EvolutionTargetSkill,
		TargetID:        s.SkillID,
		ActionType:      legacyTriggerToActionType(s.Type),
		TriggerSource:   "health",
		TriggerReason:   s.TriggerReason,
		Status:          stringToLegacyStatus(s.Status),
		Priority:        1,
		DraftBody:       s.DraftSkillBody,
		LifecycleStatus: string(s.LifecycleStatus),
		SandboxPassed:   s.SandboxPassed,
		SandboxResult:   s.SandboxResult,
		CreatedAt:       s.CreatedAt,
		ApprovedBy:      s.ApprovedBy,
	}
}

// unifiedToLegacySuggestionPtr converts a UnifiedEvolutionSuggestion to a *SkillEvolutionSuggestion
// for bridging reads from the unified store.
func unifiedToLegacySuggestionPtr(u *UnifiedEvolutionSuggestion) *SkillEvolutionSuggestion {
	return &SkillEvolutionSuggestion{
		ID:              u.ID,
		SkillID:         u.TargetID,
		Type:            actionTypeToLegacyType(u.ActionType),
		Status:          stringToLegacySuggestionStatus(u.Status),
		TriggerReason:   u.TriggerReason,
		DraftSkillBody:  u.DraftBody,
		LifecycleStatus: stringToLegacyLifecycle(u.LifecycleStatus),
		SandboxPassed:   u.SandboxPassed,
		SandboxResult:   u.SandboxResult,
		ApprovedBy:      u.ApprovedBy,
		CreatedAt:       u.CreatedAt,
	}
}

// legacyTriggerToActionType maps a legacy EvolutionSuggestionType to a UnifiedEvolutionActionType.
func legacyTriggerToActionType(t EvolutionSuggestionType) EvolutionActionType {
	switch t {
	case EvoSuggestionFixFailure:
		return EvolutionActionImprove
	case EvoSuggestionBoostEfficiency:
		return EvolutionActionImprove
	case EvoSuggestionMergeDuplicate:
		return EvolutionActionMerge
	case EvoSuggestionCreateSkill:
		return EvolutionActionCreate
	default:
		return EvolutionActionImprove
	}
}

// actionTypeToLegacyType maps a UnifiedEvolutionActionType back to a legacy EvolutionSuggestionType.
func actionTypeToLegacyType(a EvolutionActionType) EvolutionSuggestionType {
	switch a {
	case EvolutionActionImprove:
		return EvoSuggestionFixFailure
	case EvolutionActionMerge:
		return EvoSuggestionMergeDuplicate
	case EvolutionActionCreate:
		return EvoSuggestionCreateSkill
	default:
		return EvoSuggestionFixFailure
	}
}

// stringToLegacyStatus maps a unified status string to a legacy EvolutionSuggestionStatus.
func stringToLegacyStatus(s EvolutionSuggestionStatus) string {
	return string(s)
}

// stringToLegacySuggestionStatus converts a unified status string to a legacy EvolutionSuggestionStatus.
func stringToLegacySuggestionStatus(s string) EvolutionSuggestionStatus {
	switch s {
	case "pending":
		return EvoSuggestionPending
	case "approved":
		return EvoSuggestionApproved
	case "rejected":
		return EvoSuggestionRejected
	case "applied":
		return EvoSuggestionApplied
	default:
		return EvoSuggestionPending
	}
}

// stringToLegacyLifecycle converts a unified lifecycle string to a legacy EvolutionLifecycleStatus.
func stringToLegacyLifecycle(s string) EvolutionLifecycleStatus {
	switch s {
	case "draft":
		return EvoLifecycleDraft
	case "validating":
		return EvoLifecycleValidating
	case "ready":
		return EvoLifecycleReady
	case "applied":
		return EvoLifecycleApplied
	default:
		return EvoLifecycleDraft
	}
}
