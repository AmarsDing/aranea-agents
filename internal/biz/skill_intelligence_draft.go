package biz

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

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

	pending, err := uc.unifiedStore.ListByTarget(ctx, string(EvolutionTargetSkill), skillID, "", "pending", 10, 0)
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SKILL_INTELLIGENCE")
	}
	for i := range pending {
		s := &pending[i]
		if s.LifecycleStatus != "draft" || s.DraftBody != "" {

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

	if err := uc.unifiedStore.UpdateLifecycleStatus(ctx, s.ID, "validating"); err != nil {
		uc.lg.Warn("validateCuratorSuggestionUnified: UpdateLifecycleStatus(validating) failed",
			loggateway.StepID("skill_intelligence.curator_validate"),
			loggateway.Str("suggestion_id", s.ID),
			loggateway.Err(err))
	}

	passed, resultJSON := uc.runSandboxCheck(ctx, s.ID, s.TargetID, draft)
	if err := uc.unifiedStore.UpdateSandboxResult(ctx, s.ID, passed, resultJSON); err != nil {
		uc.lg.Warn("validateCuratorSuggestionUnified: UpdateSandboxResult failed",
			loggateway.StepID("skill_intelligence.curator_validate"),
			loggateway.Str("suggestion_id", s.ID),
			loggateway.Err(err))
	}

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
