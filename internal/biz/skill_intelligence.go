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

// Expiration of stale pending suggestions is owned by
// SkillEvolutionOrchestrator.ExpirePending (driven by
// EvolutionOrchestratorWorker), which sets status='expired' via the unified
// state machine. The curator worker no longer expires anything itself.

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

	// Cross-pipeline dedup: skip if another pipeline already has a pending
	// suggestion for this skill.
	if uc.orchestrator != nil {
		hasPending, err := uc.orchestrator.HasPendingForTarget(ctx, "skill", skillID)
		if err == nil && hasPending {
			uc.lg.Debug("CheckEvolutionTriggers: skipped, pending evolution already exists via orchestrator",
				loggateway.StepID("skill_intelligence.evo_trigger"),
				loggateway.Str("skill_id", skillID))
			return nil, nil
		}
	}

	// Check cooldown: if a recent suggestion exists within the cooldown period, skip.
	// F9: only active lifecycle states (pending/approved/applied) count — a
	// rejected or expired suggestion must not suppress re-triggering.
	latestUnified, uErr := uc.unifiedStore.GetLatestByTarget(ctx, "skill", skillID)
	if uErr == nil && latestUnified != nil && latestUnified.CountsForCooldown() {
		cooldownEnd := latestUnified.CreatedAt.Add(EvoTriggerCooldownHours * time.Hour)
		if time.Now().UTC().Before(cooldownEnd) {
			return nil, nil
		}
	} else if uErr != nil {
		// Unified storage read failed — conservatively skip this trigger
		// to avoid bypassing the cooldown period.
		uc.lg.Warn("CheckEvolutionTriggers: unified storage read failed, skipping trigger to avoid cooldown bypass",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(uErr))
		return nil, nil
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

	// Write to the unified store (single physical storage, A6).
	// P1: 基线成功率随建议落库（7d 优先，无 7d 数据时用 30d 值），供下一
	// 周期 AttributeLastEvolution 做有效性裁决。
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
	// Step 1: Trigger detection.
	suggestion, err := uc.CheckEvolutionTriggers(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if suggestion == nil {
		return nil, nil
	}

	// Step 2: Generate draft body (LLM evolver first, rule-based fallback).
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

	// Step 3: Set lifecycle to validating.
	if lcErr := uc.unifiedStore.UpdateLifecycleStatus(ctx, suggestion.ID, "validating"); lcErr != nil {
		uc.lg.Warn("RunCuratorFlow: UpdateLifecycleStatus(validating) failed",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(lcErr))
	}
	suggestion.LifecycleStatus = EvoLifecycleValidating

	// Step 4: Sandbox validation — use GateVerifier if available, otherwise rule-based fallback.
	passed, resultJSON := uc.runSandboxCheck(ctx, suggestion.ID, suggestion.SkillID, suggestion.DraftSkillBody)
	if sbErr := uc.unifiedStore.UpdateSandboxResult(ctx, suggestion.ID, passed, resultJSON); sbErr != nil {
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
	// drafts that passed validation are promoted to "ready".
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

// generateDraft is the unified draft-generation entry: LLM evolver first
// (Curator role), falling back to the rule-based template on any failure.
// Returns (draft, llmGenerated) — llmGenerated gates lifecycle promotion to
// "ready" (rule-based templates always stay "draft" per curator semantics).
//
// P1: draft 生成前组装归因裁决（上一次 applied 的有效性）与近期失败
// trace，注入 Curator prompt；delta 模式下实际应用的 ops 落 delta_ops
// metadata 账（供下一周期归因提取 AffectedRuleIDs）。
func (uc *SkillIntelligenceUsecase) generateDraft(ctx context.Context, suggestion *SkillEvolutionSuggestion) (string, bool) {
	if uc.evolver != nil {
		var deltaOps []DeltaOp
		draft, err := uc.evolver.EvolveDraft(ctx, SkillDraftInput{
			SkillID:       suggestion.SkillID,
			SuggestType:   suggestion.Type,
			TriggerReason: suggestion.TriggerReason,
			Attribution:   uc.AttributeLastEvolution(ctx, suggestion.SkillID),
			Traces:        uc.collectFailureTraces(ctx, suggestion.SkillID),
			DeltaOpsOut:   &deltaOps,
		})
		if err == nil {
			uc.persistDeltaOps(ctx, suggestion.ID, deltaOps)
			return draft, true
		}
		uc.lg.Warn("generateDraft: LLM evolver failed, falling back to rule-based",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("skill_id", suggestion.SkillID),
			loggateway.Err(err))
	} else {
		// F8 (P-evo-3)：evolver 未配置走模板不再是静默降级——Warn 日志 +
		// draft_origin=rule_template 落库（见 persistDraftOrigin）。
		uc.lg.Warn("generateDraft: LLM evolver not configured, using rule-based template",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("skill_id", suggestion.SkillID))
	}
	return uc.generateRuleBasedDraft(suggestion), false
}

// draftOriginValue maps the llmGenerated flag to the persisted origin value.
func draftOriginValue(llmGenerated bool) string {
	if llmGenerated {
		return DraftOriginLLM
	}
	return DraftOriginRuleTemplate
}

// persistDraftOrigin records how the draft was produced (F8) so the API view
// can expose template degradation instead of hiding it.
func (uc *SkillIntelligenceUsecase) persistDraftOrigin(ctx context.Context, suggestionID string, llmGenerated bool) {
	if err := uc.unifiedStore.UpdateMetadataKey(ctx, suggestionID, EvoMetaDraftOrigin, draftOriginValue(llmGenerated)); err != nil {
		uc.lg.Warn("persistDraftOrigin: UpdateMetadataKey failed",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
	}
}

// collectFailureTraces assembles trace-level observation evidence: the latest
// 3 failed experience reports for the skill (scanned from the 10 most recent).
// nil-safe: any read failure yields nil traces (Curator proceeds with
// aggregate evidence only).
func (uc *SkillIntelligenceUsecase) collectFailureTraces(ctx context.Context, skillID string) []TraceSnippet {
	if uc.reporter == nil {
		return nil
	}
	reports, err := uc.reporter.GetExperienceReports(ctx, skillID, 10, 0)
	if err != nil {
		uc.lg.Warn("collectFailureTraces: GetExperienceReports failed",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil
	}
	var traces []TraceSnippet
	for _, r := range reports {
		if r.IsSuccess {
			continue
		}
		traces = append(traces, TraceSnippet{
			FailureTags:       r.FailureTags,
			FlowSummary:       r.FlowSummary,
			RootCauseAnalysis: r.RootCauseAnalysis,
			CreatedAt:         r.CreatedAt,
		})
		if len(traces) >= 3 {
			break
		}
	}
	return traces
}

// persistDeltaOps stores the actually-applied delta op sequence into the
// suggestion metadata (delta mode only). The value goes through
// UpdateMetadataKey, which stores it as a JSON string (readers use
// MetaString + second unmarshal).
func (uc *SkillIntelligenceUsecase) persistDeltaOps(ctx context.Context, suggestionID string, ops []DeltaOp) {
	if len(ops) == 0 || uc.unifiedStore == nil {
		return
	}
	raw, err := json.Marshal(ops)
	if err != nil {
		return
	}
	if err := uc.unifiedStore.UpdateMetadataKey(ctx, suggestionID, EvoMetaDeltaOps, string(raw)); err != nil {
		uc.lg.Warn("persistDeltaOps: UpdateMetadataKey failed",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
	}
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

// Sandbox validator identifiers recorded in the sandbox_result JSON payload
// (F10): the trigger path (GateVerifier, 5-dimension gate) and the approve
// path (SandboxRunner, 3 rule checks + optional code execution) run different
// check sets, so the payload must name its producer — otherwise "passed" is
// ambiguous across paths.
const (
	// SandboxValidatorGateVerifier marks payloads produced via SkillGateVerifier.
	SandboxValidatorGateVerifier = "gate_verifier"
	// SandboxValidatorSandboxRunner marks payloads produced by the service-layer
	// SandboxRunner (ValidateSuggestion / RunSandbox).
	SandboxValidatorSandboxRunner = "sandbox_runner"
	// SandboxValidatorRuleBased marks payloads from the biz rule-based fallback
	// used when no GateVerifier is wired.
	SandboxValidatorRuleBased = "rule_based"
)

// runSandboxCheck executes sandbox verification for a suggestion draft:
// GateVerifier when wired (A7), otherwise the rule-based fallback. It returns
// the verdict plus a JSON result payload suitable for persistence. Gate errors
// degrade to passed=false so the lifecycle can still advance deterministically.
func (uc *SkillIntelligenceUsecase) runSandboxCheck(ctx context.Context, suggestionID, skillID, draftBody string) (bool, json.RawMessage) {
	if uc.gate != nil {
		var passed bool
		gateResult, gateErr := uc.gate.Verify(ctx, skillID, draftBody, nil)
		if gateErr != nil {
			uc.lg.Warn("runSandboxCheck: GateVerifier failed",
				loggateway.StepID("skill_intelligence.curator_flow"),
				loggateway.Str("suggestion_id", suggestionID),
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
		return passed, uc.marshalSandboxResult(suggestionID, passed, checkResults, SandboxValidatorGateVerifier)
	}
	passed := draftBody != "" && len(draftBody) < 10000 && skillID != ""
	return passed, uc.marshalSandboxResult(suggestionID, passed, []map[string]any{
		{"name": "draft_body_not_empty", "passed": draftBody != ""},
		{"name": "draft_body_length", "passed": len(draftBody) < 10000},
		{"name": "skill_id_valid", "passed": skillID != ""},
	}, SandboxValidatorRuleBased)
}

// marshalSandboxResult builds the JSON sandbox-result payload. Marshal failures
// degrade to a minimal fallback payload so persistence never fails on encoding.
func (uc *SkillIntelligenceUsecase) marshalSandboxResult(suggestionID string, passed bool, checks []map[string]any, validator string) json.RawMessage {
	raw, err := json.Marshal(map[string]any{
		"passed":    passed,
		"validator": validator,
		"checks":    checks,
		"message": func() string {
			if passed {
				return "All validation checks passed"
			}
			return "Some validation checks failed"
		}(),
	})
	if err != nil {
		uc.lg.Warn("marshalSandboxResult: marshal failed, using fallback",
			loggateway.StepID("skill_intelligence.curator_flow"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
		return json.RawMessage(`{"error":"marshal failed"}`)
	}
	return raw
}

// ValidatePendingSuggestionsForSkill verifies orchestrator-created pending
// suggestions (lifecycle=draft) for a skill: draft generation → validating →
// sandbox verification → lifecycle update. Triggering is the exclusive
// responsibility of EvolutionOrchestratorWorker (unified entry); this method
// only runs the verification half of the curator pipeline.
//
// Orchestrator-created suggestions live only in the unified store, so writes
// here are unified-only (no legacy bridge).
func (uc *SkillIntelligenceUsecase) ValidatePendingSuggestionsForSkill(ctx context.Context, skillID string) error {
	if uc.unifiedStore == nil {
		return nil
	}
	skillID, err := requireNonEmpty(skillID, "SKILL_INTELLIGENCE", "skill_id")
	if err != nil {
		return err
	}
	// The orchestrator guarantees at most one pending suggestion per target,
	// but list a small page defensively.
	pending, err := uc.unifiedStore.ListByTarget(ctx, string(EvolutionTargetSkill), skillID, "", "pending", 10, 0)
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SKILL_INTELLIGENCE")
	}
	for i := range pending {
		s := &pending[i]
		if s.LifecycleStatus != "draft" || s.DraftBody != "" {
			// Already validated (or validating) — skip to keep the operation idempotent.
			continue
		}
		if err := uc.validateCuratorSuggestionUnified(ctx, s); err != nil {
			uc.lg.Warn("ValidatePendingSuggestionsForSkill: validate failed",
				loggateway.StepID("skill_intelligence.curator_validate"),
				loggateway.Str("suggestion_id", s.ID),
				loggateway.Err(err))
		}
	}
	return nil
}

// validateCuratorSuggestionUnified runs the verification half of the curator
// pipeline for an orchestrator-created (unified-only) suggestion:
// draft generation → validating → sandbox verification → lifecycle update.
func (uc *SkillIntelligenceUsecase) validateCuratorSuggestionUnified(ctx context.Context, s *UnifiedEvolutionSuggestion) error {
	// Step 1: Generate draft body (LLM evolver first, rule-based fallback).
	// SuggestType derives from trigger source: success → success_pattern
	// (consolidation), everything else → fix_failure template (F3).
	draft, llmGenerated := uc.generateDraft(ctx, &SkillEvolutionSuggestion{
		ID:            s.ID,
		SkillID:       s.TargetID,
		Type:          suggestTypeForUnified(s),
		TriggerReason: s.TriggerReason,
	})
	if err := uc.unifiedStore.UpdateDraftBody(ctx, s.ID, draft); err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SKILL_INTELLIGENCE")
	}
	uc.persistDraftOrigin(ctx, s.ID, llmGenerated)

	// Step 2: Set lifecycle to validating.
	if err := uc.unifiedStore.UpdateLifecycleStatus(ctx, s.ID, "validating"); err != nil {
		uc.lg.Warn("validateCuratorSuggestionUnified: UpdateLifecycleStatus(validating) failed",
			loggateway.StepID("skill_intelligence.curator_validate"),
			loggateway.Str("suggestion_id", s.ID),
			loggateway.Err(err))
	}

	// Step 3: Sandbox verification.
	passed, resultJSON := uc.runSandboxCheck(ctx, s.ID, s.TargetID, draft)
	if err := uc.unifiedStore.UpdateSandboxResult(ctx, s.ID, passed, resultJSON); err != nil {
		uc.lg.Warn("validateCuratorSuggestionUnified: UpdateSandboxResult failed",
			loggateway.StepID("skill_intelligence.curator_validate"),
			loggateway.Str("suggestion_id", s.ID),
			loggateway.Err(err))
	}

	// Step 4: Rule-based template drafts always return to "draft" (needs human
	// editing); LLM-generated drafts that passed sandbox are promoted to
	// "ready" — same semantics as RunCuratorFlow.
	finalLifecycle := "draft"
	if llmGenerated && passed {
		finalLifecycle = "ready"
	}
	if err := uc.unifiedStore.UpdateLifecycleStatus(ctx, s.ID, finalLifecycle); err != nil {
		uc.lg.Warn("validateCuratorSuggestionUnified: UpdateLifecycleStatus(final) failed",
			loggateway.StepID("skill_intelligence.curator_validate"),
			loggateway.Str("suggestion_id", s.ID),
			loggateway.Err(err))
	}
	return nil
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

	draft, llmGenerated := uc.generateDraft(ctx, suggestion)

	if err := uc.UpdateSuggestionDraftBody(ctx, suggestionID, draft); err != nil {
		uc.lg.Warn("GenerateDraftForSuggestion: UpdateSuggestionDraftBody failed",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
		return "", err
	}
	uc.persistDraftOrigin(ctx, suggestionID, llmGenerated)

	if lcErr := uc.UpdateSuggestionLifecycleStatus(ctx, suggestionID, EvoLifecycleDraft); lcErr != nil {
		uc.lg.Warn("GenerateDraftForSuggestion: UpdateSuggestionLifecycleStatus failed",
			loggateway.StepID("skill_intelligence.generate_draft"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(lcErr))
	}

	return draft, nil
}

// CreateSuggestion creates a skill-scoped evolution suggestion (L2 view)
// in the unified store (A6).
func (uc *SkillIntelligenceUsecase) CreateSuggestion(ctx context.Context, suggestion SkillEvolutionSuggestion) error {
	if uc.unifiedStore == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.unifiedStore.Create(ctx, unifiedToLegacySuggestion(&suggestion))
}

// GetEvolutionSuggestion returns a single evolution suggestion by ID.
// Reads the unified store and converts to the legacy L2 view (A6).
// Returns (nil, nil) when not found.
func (uc *SkillIntelligenceUsecase) GetEvolutionSuggestion(ctx context.Context, id string) (*SkillEvolutionSuggestion, error) {
	if uc.unifiedStore == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	unified, err := uc.unifiedStore.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if unified == nil {
		return nil, nil
	}
	return unifiedToLegacySuggestionPtr(unified), nil
}

// UpdateSuggestionDraftBody updates the draft skill body of an evolution suggestion.
func (uc *SkillIntelligenceUsecase) UpdateSuggestionDraftBody(ctx context.Context, id string, draftBody string) error {
	if uc.unifiedStore == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.unifiedStore.UpdateDraftBody(ctx, id, draftBody)
}

// UpdateSuggestionSandboxResult updates the sandbox validation result of an evolution suggestion.
func (uc *SkillIntelligenceUsecase) UpdateSuggestionSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	if uc.unifiedStore == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.unifiedStore.UpdateSandboxResult(ctx, id, passed, result)
}

// UpdateSuggestionLifecycleStatus updates the lifecycle status of an evolution suggestion.
func (uc *SkillIntelligenceUsecase) UpdateSuggestionLifecycleStatus(ctx context.Context, id string, lifecycleStatus EvolutionLifecycleStatus) error {
	if uc.unifiedStore == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.unifiedStore.UpdateLifecycleStatus(ctx, id, string(lifecycleStatus))
}

// ListEvolutionSuggestions lists evolution suggestions for a skill, optionally filtered by status.
// Reads the unified store and converts to the legacy L2 view (A6).
func (uc *SkillIntelligenceUsecase) ListEvolutionSuggestions(ctx context.Context, skillID string, status EvolutionSuggestionStatus, limit, offset int) ([]SkillEvolutionSuggestion, error) {
	if uc.unifiedStore == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	unifiedList, err := uc.unifiedStore.ListByTarget(ctx, "skill", skillID, evolutionCallerWorkspace(ctx), string(status), limit, offset)
	if err != nil {
		return nil, err
	}
	result := make([]SkillEvolutionSuggestion, len(unifiedList))
	for i, u := range unifiedList {
		result[i] = *unifiedToLegacySuggestionPtr(&u)
	}
	return result, nil
}

// CountEvolutionSuggestions returns the total count of evolution suggestions for a skill, optionally filtered by status.
func (uc *SkillIntelligenceUsecase) CountEvolutionSuggestions(ctx context.Context, skillID string, status EvolutionSuggestionStatus) (int, error) {
	if uc.unifiedStore == nil {
		return 0, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	return uc.unifiedStore.CountByTarget(ctx, "skill", skillID, evolutionCallerWorkspace(ctx), string(status))
}

// ApproveSuggestion approves a pending evolution suggestion. The transition is
// validated against the unified state machine and persisted via CAS so that
// only pending → approved can succeed (concurrent or repeated transitions
// return Conflict instead of silently regressing state).
func (uc *SkillIntelligenceUsecase) ApproveSuggestion(ctx context.Context, id, approvedBy string) error {
	if uc.unifiedStore == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.transitionSuggestionStatus(ctx, id, UnifiedEvolutionEventApprove, approvedBy, "")
}

// RejectSuggestion rejects a pending evolution suggestion. Same state-machine +
// CAS discipline as ApproveSuggestion.
func (uc *SkillIntelligenceUsecase) RejectSuggestion(ctx context.Context, id, rejectedBy, reason string) error {
	if uc.unifiedStore == nil {
		return apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	return uc.transitionSuggestionStatus(ctx, id, UnifiedEvolutionEventReject, rejectedBy, reason)
}

// transitionSuggestionStatus is the shared approve/reject path: read current
// state → state-machine validation → CAS persistence.
func (uc *SkillIntelligenceUsecase) transitionSuggestionStatus(ctx context.Context, id string, event UnifiedEvolutionEvent, actor, reason string) error {
	row, err := uc.unifiedStore.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return apierror.NotFound("SKILL_INTELLIGENCE", "suggestion not found: %s", id)
	}
	next, err := uc.unifiedSM.Transition(UnifiedEvolutionState(row.Status), event)
	if err != nil {
		return apierror.BadRequest("SKILL_INTELLIGENCE", "invalid suggestion transition from %s on event %s", row.Status, event)
	}
	ok, err := uc.unifiedStore.UpdateStatusCAS(ctx, id,
		[]string{string(UnifiedEvolutionStatePending)}, string(next), actor, reason)
	if err != nil {
		return err
	}
	if !ok {
		return apierror.Conflict("SKILL_INTELLIGENCE", "suggestion %s status changed concurrently; retry", id)
	}
	return nil
}

// ApplyApprovedSuggestion runs the Reload stage for an approved evolution
// suggestion: it registers the approved draft as a new skill version and
// marks the suggestion applied. It is a guarded no-op unless the suggestion
// is approved + lifecycle=ready + sandbox_passed with a non-empty draft, so
// callers may invoke it unconditionally after any draft/validation attempt.
//
// State transitions are validated through the two explicit state machines
// (AS-FSM-01): status approved --apply--> applied (UnifiedEvolutionStateMachine)
// and lifecycle ready --apply--> applied (EvolutionLifecycleStateMachine).
//
// On Reload failure the error is returned and the suggestion stays ready —
// no automatic retry, to avoid duplicate version registration (manual retry
// is possible by re-approving / re-invoking).
func (uc *SkillIntelligenceUsecase) ApplyApprovedSuggestion(ctx context.Context, suggestionID string) error {
	if uc.reloader == nil || uc.unifiedStore == nil {
		return nil
	}
	suggestion, err := uc.GetEvolutionSuggestion(ctx, suggestionID)
	if err != nil {
		return err
	}
	if suggestion == nil {
		return apierror.NotFound("SKILL_INTELLIGENCE", "suggestion not found: %s", suggestionID)
	}

	// Guards: draft presence and sandbox pass are plain field checks; the
	// status/lifecycle apply-ability is checked via the state machines below.
	if !suggestion.SandboxPassed || strings.TrimSpace(suggestion.DraftSkillBody) == "" {
		return nil
	}
	nextStatus, err := uc.unifiedSM.Transition(UnifiedEvolutionState(suggestion.Status), UnifiedEvolutionEventApply)
	if err != nil {
		return nil // not in approved state → nothing to apply
	}
	nextLifecycle, err := uc.lifecycleSM.Transition(suggestion.LifecycleStatus, EvoLifecycleEventApply)
	if err != nil {
		return nil // not in ready state → nothing to apply
	}

	reason := strings.TrimSpace(suggestion.EvolutionReason)
	if reason == "" {
		reason = fmt.Sprintf("evolution: %s (%s)", suggestion.Type, suggestion.TriggerReason)
	}
	if err := uc.reloader.Reload(ctx, suggestion.SkillID, suggestion.DraftSkillBody, suggestion.ParentVersionID, reason); err != nil {
		uc.lg.Error("ApplyApprovedSuggestion: Reload failed, suggestion stays ready",
			loggateway.StepID("skill_intelligence.apply"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Str("skill_id", suggestion.SkillID),
			loggateway.Err(err))
		return err
	}

	if err := uc.unifiedStore.UpdateLifecycleStatus(ctx, suggestionID, string(nextLifecycle)); err != nil {
		uc.lg.Warn("ApplyApprovedSuggestion: UpdateLifecycleStatus(applied) failed",
			loggateway.StepID("skill_intelligence.apply"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
	}
	if err := uc.unifiedStore.UpdateStatus(ctx, suggestionID, string(nextStatus), "system", ""); err != nil {
		uc.lg.Warn("ApplyApprovedSuggestion: UpdateStatus(applied) failed",
			loggateway.StepID("skill_intelligence.apply"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
		return err
	}
	uc.lg.Info("ApplyApprovedSuggestion: suggestion applied",
		loggateway.StepID("skill_intelligence.apply"),
		loggateway.Str("suggestion_id", suggestionID),
		loggateway.Str("skill_id", suggestion.SkillID))
	return nil
}

// ── Bridge functions: Unified ↔ Legacy ────────────────────────────────────────

// unifiedToLegacySuggestion converts a SkillEvolutionSuggestion to a UnifiedEvolutionSuggestion
// for writes to the unified store. The exact legacy type is preserved in
// metadata (action_type mapping is lossy), matching the backfill mapping (A6).
func unifiedToLegacySuggestion(s *SkillEvolutionSuggestion) UnifiedEvolutionSuggestion {
	metadata, _ := json.Marshal(map[string]string{
		EvoMetaLegacyType: string(s.Type),
	})
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
		Metadata:        metadata,
		CreatedAt:       s.CreatedAt,
		ApprovedBy:      s.ApprovedBy,
	}
}

// unifiedToLegacySuggestionPtr converts a UnifiedEvolutionSuggestion to a *SkillEvolutionSuggestion
// for bridging reads from the unified store.
// Resolved/rejection fields and Curator tracking fields are reconstructed from
// metadata keys merged by UpdateStatus and the 20261111 backfill (A6),
// mirroring the legacy L2 columns. The legacy suggestion type prefers the
// exact legacy_type metadata (action_type mapping is lossy).
func unifiedToLegacySuggestionPtr(u *UnifiedEvolutionSuggestion) *SkillEvolutionSuggestion {
	var resolvedAt *time.Time
	if raw := u.MetaString(EvoMetaResolvedAt); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			resolvedAt = &t
		}
	}
	suggestionType := actionTypeToLegacyType(u.ActionType)
	if lt := u.MetaString(EvoMetaLegacyType); lt != "" {
		suggestionType = EvolutionSuggestionType(lt)
	}
	var sourceReportIDs []string
	if raw := u.MetaRaw(EvoMetaSourceReportIDs); len(raw) > 0 {
		_ = json.Unmarshal(raw, &sourceReportIDs)
	}
	return &SkillEvolutionSuggestion{
		ID:              u.ID,
		SkillID:         u.TargetID,
		Type:            suggestionType,
		Status:          stringToLegacySuggestionStatus(u.Status),
		SourceReportIDs: sourceReportIDs,
		TriggerReason:   u.TriggerReason,
		DraftSkillBody:  u.DraftBody,
		DraftVersionID:  u.MetaString(EvoMetaDraftVersionID),
		LifecycleStatus: stringToLegacyLifecycle(u.LifecycleStatus),
		SandboxPassed:   u.SandboxPassed,
		SandboxResult:   u.SandboxResult,
		PreVerifyResult: u.MetaRaw(EvoMetaPreVerifyResult),
		ApprovedBy:      u.ApprovedBy,
		RejectedBy:      u.MetaString(EvoMetaRejectedBy),
		RejectionReason: u.MetaString(EvoMetaRejectionReason),
		ResolvedAt:      resolvedAt,
		CreatedAt:       u.CreatedAt,
		ParentVersionID: u.MetaString(EvoMetaParentVersionID),
		EvolutionReason: u.MetaString(EvoMetaEvolutionReason),
		DraftOrigin:     u.MetaString(EvoMetaDraftOrigin),
	}
}

// suggestTypeForUnified derives the evolver SuggestType from a unified
// suggestion's trigger source (F3): success-triggered suggestions get the
// success_pattern (consolidation) prompt branch; all others use fix_failure.
func suggestTypeForUnified(s *UnifiedEvolutionSuggestion) EvolutionSuggestionType {
	if s != nil && s.TriggerSource == SuccessTriggerSource {
		return EvoSuggestionSuccessPattern
	}
	return EvoSuggestionFixFailure
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
		return EvoSuggestionBoostEfficiency
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
