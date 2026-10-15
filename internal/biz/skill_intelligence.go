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
// Evolution suggestions are served exclusively by the unified store (A6);
// legacy-view conversion happens at the method boundary.
type SkillIntelligenceUsecase struct {
	scorer       *SkillScoringUsecase
	reporter     *SkillReportUsecase
	unifiedStore UnifiedEvolutionStore
	aggregator   SkillHealthAggregator
	orchestrator *SkillEvolutionOrchestrator
	gate         SkillGateVerifier
	evolver      SkillDraftEvolver
	reloader     SkillReloader
	registrar    SkillRegistrationPort
	lg           loggateway.Logger

	// AS-FSM-01: suggestion status (unifiedSM) and draft lifecycle (lifecycleSM)
	// transitions are validated by explicit state machines.
	unifiedSM   *UnifiedEvolutionStateMachine
	lifecycleSM *EvolutionLifecycleStateMachine

	unanalyzedReader SkillInvocationUnanalyzedReader
}

// SkillIntelligenceConfig holds optional dependencies for SkillIntelligenceUsecase.
type SkillIntelligenceConfig struct {
	Scorer           *SkillScoringUsecase
	Reporter         *SkillReportUsecase
	Orchestrator     *SkillEvolutionOrchestrator
	UnanalyzedReader SkillInvocationUnanalyzedReader
	Gate             SkillGateVerifier
	// Evolver generates LLM-based skill drafts (Curator role). nil → rule-based
	// template fallback everywhere (P0: nil-safe degradation).
	Evolver SkillDraftEvolver
	// Reloader registers an approved evolved draft as a new skill version
	// (Reload stage). nil → approval pipeline stops at lifecycle=ready with
	// no version write (P0: nil-safe degradation).
	Reloader SkillReloader
	// Registrar mounts an approved agent create_skill suggestion's draft as a
	// skill on the target agent (ADR-3 auto-register). nil → agent suggestions
	// stay approved with no registration (nil-safe degradation).
	Registrar SkillRegistrationPort
}

// NewSkillIntelligenceUsecase constructs a SkillIntelligenceUsecase.
func NewSkillIntelligenceUsecase(
	scorer *SkillScoringUsecase,
	reporter *SkillReportUsecase,
	unifiedStore UnifiedEvolutionStore,
	aggregator SkillHealthAggregator,
	lg loggateway.Logger,
	opts ...SkillIntelligenceConfig,
) *SkillIntelligenceUsecase {
	uc := &SkillIntelligenceUsecase{
		scorer:       scorer,
		reporter:     reporter,
		unifiedStore: unifiedStore,
		aggregator:   aggregator,
		lg:           lg,
		unifiedSM:    NewUnifiedEvolutionStateMachine(),
		lifecycleSM:  NewEvolutionLifecycleStateMachine(),
	}
	for _, opt := range opts {
		if opt.Orchestrator != nil {
			uc.orchestrator = opt.Orchestrator
		}
		if opt.UnanalyzedReader != nil {
			uc.unanalyzedReader = opt.UnanalyzedReader
		}
		if opt.Gate != nil {
			uc.gate = opt.Gate
		}
		if opt.Evolver != nil {
			uc.evolver = opt.Evolver
		}
		if opt.Reloader != nil {
			uc.reloader = opt.Reloader
		}
		if opt.Registrar != nil {
			uc.registrar = opt.Registrar
		}
	}
	return uc
}

// SetGate wires the Gate verifier after construction. Needed because the gate
// implementation chain (GateVerifier → service.SandboxRunner → this usecase)
// is cyclic at the DI level: the usecase cannot receive the gate as a
// constructor dependency. Called once from the wire provider that assembles
// the CuratorWorker, during single-threaded app startup.
func (uc *SkillIntelligenceUsecase) SetGate(g SkillGateVerifier) {
	if g != nil {
		uc.gate = g
	}
}

// ExperienceReportListResult holds the result of a filtered experience report query,
// including pagination total and aggregate statistics.
type ExperienceReportListResult struct {
	Reports          []ExperienceReport
	TotalCount       int
	FailureTagCounts []FailureTagCount
	RootCauseReports []ExperienceReport
	// Stats 为筛选条件下的成功/失败计数与平均评分；聚合失败时为 nil（降级，不阻断列表）。
	Stats *ExperienceReportStats
}

// AnalyzeInvocation delegates to SkillScoringUsecase.
func (uc *SkillIntelligenceUsecase) AnalyzeInvocation(ctx context.Context, inv SkillInvocationWrite) (isSuccess bool, failureTags []string) {
	return uc.scorer.AnalyzeInvocation(ctx, inv)
}

// ScoreSkill delegates to SkillScoringUsecase.
func (uc *SkillIntelligenceUsecase) ScoreSkill(ctx context.Context, skillID string) (int, error) {
	return uc.scorer.ScoreSkill(ctx, skillID)
}

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

// CheckEvolutionTriggers checks if a skill meets the conditions for generating
// an evolution suggestion. Returns a new suggestion if triggered, or nil if not.
//
// Trigger conditions (any one suffices):
//  1. 30d failure rate > 30% (existing)
//  2. 7d success rate < 60% (Curator Agent)
//  3. Same failure tag >= 5 times in 7d (Curator Agent)
//  4. Skill score < 60 (existing)
func (uc *SkillIntelligenceUsecase) CheckEvolutionTriggers(ctx context.Context, skillID string) (*SkillEvolutionSuggestion, error) {
	if uc.aggregator == nil || uc.unifiedStore == nil {
		return nil, nil
	}

	skillID, err := requireNonEmpty(skillID, "SKILL_INTELLIGENCE", "skill_id")
	if err != nil {
		return nil, err
	}

	if uc.orchestrator != nil {
		hasPending, err := uc.orchestrator.HasPendingForTarget(ctx, "skill", skillID)
		if err == nil && hasPending {
			uc.lg.Debug("CheckEvolutionTriggers: skipped, pending evolution already exists via orchestrator",
				loggateway.StepID("skill_intelligence.evo_trigger"),
				loggateway.Str("skill_id", skillID))
			return nil, nil
		}
	}

	latestUnified, uErr := uc.unifiedStore.GetLatestByTarget(ctx, "skill", skillID)
	if uErr == nil && latestUnified != nil && latestUnified.CountsForCooldown() {
		cooldownEnd := latestUnified.CreatedAt.Add(EvoTriggerCooldownHours * time.Hour)
		if time.Now().UTC().Before(cooldownEnd) {
			return nil, nil
		}
	} else if uErr != nil {

		uc.lg.Warn("CheckEvolutionTriggers: unified storage read failed, skipping trigger to avoid cooldown bypass",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(uErr))
		return nil, nil
	}

	since30d := time.Now().UTC().Add(-30 * 24 * time.Hour)
	metrics, err := uc.aggregator.GetHealthMetrics(ctx, skillID, since30d)
	if err != nil {
		uc.lg.Warn("CheckEvolutionTriggers: GetHealthMetrics failed",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil, err
	}

	if metrics.InvocationCount < EvoTriggerMinInvocations {
		return nil, nil
	}

	// Determine trigger conditions. Collect all matching types to avoid
	// losing semantic information when multiple conditions fire.
	var triggerTypes []EvolutionSuggestionType
	var triggerReasons []string

	failureRate := 1.0 - metrics.SuccessRate
	if failureRate > EvoTriggerFailureRate {
		triggerTypes = append(triggerTypes, EvoSuggestionFixFailure)
		triggerReasons = append(triggerReasons, fmt.Sprintf("30d failure rate %.1f%% exceeds threshold %.1f%% (%d invocations)",
			failureRate*100, EvoTriggerFailureRate*100, metrics.InvocationCount))
	}

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
				break
			}
		}
	}

	score, scoreErr := uc.scorer.ScoreSkill(ctx, skillID)
	if scoreErr == nil && score < EvoTriggerScoreThreshold {
		triggerTypes = append(triggerTypes, EvoSuggestionBoostEfficiency)
		triggerReasons = append(triggerReasons, fmt.Sprintf("Skill score %d below threshold %d", score, EvoTriggerScoreThreshold))
	}

	if len(triggerTypes) == 0 {
		return nil, nil
	}

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

	baseline := metrics.SuccessRate
	if err7d == nil && metrics7d.InvocationCount > 0 {
		baseline = metrics7d.SuccessRate
	}
	metadata, _ := json.Marshal(map[string]any{
		EvoMetaLegacyType:          string(triggerType),
		EvoMetaBaselineSuccessRate: baseline,
	})
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
		Metadata:        metadata,
		CreatedAt:       suggestion.CreatedAt,
	}
	if err := uc.unifiedStore.Create(ctx, unified); err != nil {
		uc.lg.Warn("CheckEvolutionTriggers: unified Create suggestion failed",
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

	suggestion, err := uc.CheckEvolutionTriggers(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if suggestion == nil {
		return nil, nil
	}

	draft, llmGenerated := uc.generateDraft(ctx, suggestion)

	if uErr := uc.unifiedStore.UpdateDraftBody(ctx, suggestion.ID, draft); uErr != nil {
		uc.lg.Warn("RunCuratorFlow: unified UpdateDraftBody failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(uErr))
		return nil, apierror.Wrap(uErr, apierror.CodeInternal, "SKILL_INTELLIGENCE")
	}
	uc.persistDraftOrigin(ctx, suggestion.ID, llmGenerated)
	suggestion.DraftSkillBody = draft
	suggestion.DraftOrigin = draftOriginValue(llmGenerated)

	if lcErr := uc.unifiedStore.UpdateLifecycleStatus(ctx, suggestion.ID, "validating"); lcErr != nil {
		uc.lg.Warn("RunCuratorFlow: UpdateLifecycleStatus(validating) failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(lcErr))
	}
	suggestion.LifecycleStatus = EvoLifecycleValidating

	passed, resultJSON := uc.runSandboxCheck(ctx, suggestion.ID, suggestion.SkillID, suggestion.DraftSkillBody)
	if sbErr := uc.unifiedStore.UpdateSandboxResult(ctx, suggestion.ID, passed, resultJSON); sbErr != nil {
		uc.lg.Warn("RunCuratorFlow: UpdateSandboxResult failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(sbErr))
	}
	suggestion.SandboxPassed = passed
	suggestion.SandboxResult = resultJSON

	lifecycleStatus := EvoLifecycleDraft
	if llmGenerated && passed {
		lifecycleStatus = EvoLifecycleReady
	}
	if lcErr := uc.unifiedStore.UpdateLifecycleStatus(ctx, suggestion.ID, string(lifecycleStatus)); lcErr != nil {
		uc.lg.Warn("RunCuratorFlow: UpdateLifecycleStatus(final) failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(lcErr))
	}
	suggestion.LifecycleStatus = lifecycleStatus

	return suggestion, nil
}
