package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

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
	return unifiedToLegacySuggestionPtr(uc.lg, unified), nil
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

// ListEvolutionSuggestions lists evolution suggestions for a target,
// optionally filtered by status. targetType is "skill" (default) or "agent"
// (ADR-3); agent rows are narrowed to action_type=create_skill so L3
// evolve_agent suggestions sharing the same target_type stay out of the L1 view.
// Reads the unified store and converts to the legacy L2 view (A6).
func (uc *SkillIntelligenceUsecase) ListEvolutionSuggestions(ctx context.Context, targetType, targetID string, status EvolutionSuggestionStatus, limit, offset int) ([]SkillEvolutionSuggestion, error) {
	if uc.unifiedStore == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	targetType = normalizeEvolutionTargetType(targetType)
	workspaceID := evolutionCallerWorkspace(ctx)
	var unifiedList []UnifiedEvolutionSuggestion
	var err error
	if targetType == string(EvolutionTargetAgent) {
		unifiedList, err = uc.unifiedStore.ListByTargetAndAction(ctx, targetType, targetID, string(EvolutionActionCreate), workspaceID, string(status), limit, offset)
	} else {
		unifiedList, err = uc.unifiedStore.ListByTarget(ctx, targetType, targetID, workspaceID, string(status), limit, offset)
	}
	if err != nil {
		return nil, err
	}
	result := make([]SkillEvolutionSuggestion, len(unifiedList))
	for i, u := range unifiedList {
		result[i] = *unifiedToLegacySuggestionPtr(uc.lg, &u)
	}
	return result, nil
}

// CountEvolutionSuggestions returns the total count of evolution suggestions for a target, optionally filtered by status.
func (uc *SkillIntelligenceUsecase) CountEvolutionSuggestions(ctx context.Context, targetType, targetID string, status EvolutionSuggestionStatus) (int, error) {
	if uc.unifiedStore == nil {
		return 0, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion reader not available")
	}
	targetType = normalizeEvolutionTargetType(targetType)
	workspaceID := evolutionCallerWorkspace(ctx)
	if targetType == string(EvolutionTargetAgent) {
		return uc.unifiedStore.CountByTargetAndAction(ctx, targetType, targetID, string(EvolutionActionCreate), workspaceID, string(status))
	}
	return uc.unifiedStore.CountByTarget(ctx, targetType, targetID, workspaceID, string(status))
}

// normalizeEvolutionTargetType maps an empty/unknown target type to "skill"
// (historical default before ADR-3 generalized the list RPC).
func normalizeEvolutionTargetType(targetType string) string {
	if targetType == string(EvolutionTargetAgent) {
		return string(EvolutionTargetAgent)
	}
	return string(EvolutionTargetSkill)
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
	if uc.unifiedStore == nil {
		return nil
	}
	suggestion, err := uc.GetEvolutionSuggestion(ctx, suggestionID)
	if err != nil {
		return err
	}
	if suggestion == nil {
		return apierror.NotFound("SKILL_INTELLIGENCE", "suggestion not found: %s", suggestionID)
	}

	if suggestion.TargetType == string(EvolutionTargetAgent) {
		return uc.applyAgentCreateSkill(ctx, suggestion)
	}

	if uc.reloader == nil {
		return nil
	}

	if !suggestion.SandboxPassed || strings.TrimSpace(suggestion.DraftSkillBody) == "" {
		return nil
	}
	nextStatus, err := uc.unifiedSM.Transition(UnifiedEvolutionState(suggestion.Status), UnifiedEvolutionEventApply)
	if err != nil {
		return nil
	}
	nextLifecycle, err := uc.lifecycleSM.Transition(suggestion.LifecycleStatus, EvoLifecycleEventApply)
	if err != nil {
		return nil
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

// applyAgentCreateSkill applies an approved agent create_skill suggestion
// (ADR-3): it mounts the draft as a skill on the target agent via the
// registrar, then CAS-advances the suggestion to applied. The flow is
// idempotent — an already-registered skill still advances the state so a
// prior partial success can settle. Registration failure leaves the
// suggestion approved for manual inspection/retry.
func (uc *SkillIntelligenceUsecase) applyAgentCreateSkill(ctx context.Context, suggestion *SkillEvolutionSuggestion) error {
	nextStatus, err := uc.unifiedSM.Transition(UnifiedEvolutionState(suggestion.Status), UnifiedEvolutionEventApply)
	if err != nil {
		return nil
	}
	// E2E-E1：agent 分支门禁与 skill 分支对齐——apply 必须经 lifecycle=ready
	// FSM 校验且 sandbox_passed；此前守卫只落在 skill 分支，agent 建议人工
	// approve 后即便 sandbox 未过/lifecycle=draft 也被 CAS 到 applied 并注册，
	// 产出 status=applied + lifecycle=draft + sandbox_passed=0 矛盾终态。
	nextLifecycle, err := uc.lifecycleSM.Transition(suggestion.LifecycleStatus, EvoLifecycleEventApply)
	if err != nil {
		return nil
	}
	if !suggestion.SandboxPassed {
		return nil
	}
	if uc.registrar == nil {
		return nil
	}
	name := strings.TrimSpace(suggestion.DraftName)
	body := strings.TrimSpace(suggestion.DraftSkillBody)
	if name == "" || body == "" {
		uc.lg.Warn("applyAgentCreateSkill: empty draft name/body, suggestion stays approved",
			loggateway.StepID("skill_intelligence.apply_agent"),
			loggateway.Str("suggestion_id", suggestion.ID))
		return nil
	}
	agentID := suggestion.TargetID
	exists, exErr := uc.registrar.SkillExists(ctx, agentID, name)
	if exErr != nil {
		return exErr
	}
	if !exists {
		if regErr := uc.registrar.RegisterSkill(ctx, agentID, name, suggestion.DraftSkillBody); regErr != nil {
			uc.lg.Error("applyAgentCreateSkill: RegisterSkill failed, suggestion stays approved",
				loggateway.StepID("skill_intelligence.apply_agent"),
				loggateway.Str("suggestion_id", suggestion.ID),
				loggateway.Str("agent_id", agentID),
				loggateway.Str("skill_name", name),
				loggateway.Err(regErr))
			return regErr
		}
	}
	// E2E-E1: lifecycle 先于 status 落值（与 skill 分支同序），避免 CAS 成功但
	// lifecycle 仍停在 draft 的矛盾态；lifecycle 写入失败仅告警（可重试补写），
	// status CAS 是权威门禁（并发漂移仍由 CAS 吸收）。
	if err := uc.unifiedStore.UpdateLifecycleStatus(ctx, suggestion.ID, string(nextLifecycle)); err != nil {
		uc.lg.Warn("applyAgentCreateSkill: UpdateLifecycleStatus(applied) failed",
			loggateway.StepID("skill_intelligence.apply_agent"),
			loggateway.Str("suggestion_id", suggestion.ID),
			loggateway.Err(err))
	}
	ok, err := uc.unifiedStore.UpdateStatusCAS(ctx, suggestion.ID,
		[]string{string(UnifiedEvolutionStateApproved)}, string(nextStatus), "system", "auto-registered to agent")
	if err != nil {
		return err
	}
	if !ok {
		uc.lg.Warn("applyAgentCreateSkill: status changed concurrently, registration already done",
			loggateway.StepID("skill_intelligence.apply_agent"),
			loggateway.Str("suggestion_id", suggestion.ID))
		return nil
	}
	uc.lg.Info("applyAgentCreateSkill: suggestion applied (skill registered to agent)",
		loggateway.StepID("skill_intelligence.apply_agent"),
		loggateway.Str("suggestion_id", suggestion.ID),
		loggateway.Str("agent_id", agentID),
		loggateway.Str("skill_name", name))
	return nil
}

// CreateAgentSkillSuggestion creates a pending agent create_skill suggestion
// (ADR-3): the unified entry point for the skills_butler evolve_skill tool
// after the legacy SkillProposal view was retired. patternHash/patternDesc
// land in metadata so the L1 pattern-hash dedup (GetLatestByPatternHash)
// keeps working.
func (uc *SkillIntelligenceUsecase) CreateAgentSkillSuggestion(ctx context.Context, agentID, skillName, patternDesc, patternHash string) (*SkillEvolutionSuggestion, error) {
	if uc.unifiedStore == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "suggestion writer not available")
	}
	agentID = strings.TrimSpace(agentID)
	skillName = strings.TrimSpace(skillName)
	if agentID == "" || skillName == "" {
		return nil, apierror.BadRequest("SKILL_INTELLIGENCE", "agent_id and skill_name are required")
	}
	metadata, _ := json.Marshal(map[string]string{
		EvoMetaPatternHash: patternHash,
		EvoMetaPatternDesc: patternDesc,
	})
	row := UnifiedEvolutionSuggestion{
		ID:              newAgentCatalogID(),
		TargetType:      EvolutionTargetAgent,
		TargetID:        agentID,
		ActionType:      EvolutionActionCreate,
		TriggerSource:   "skills_butler",
		TriggerReason:   patternDesc,
		Status:          string(UnifiedEvolutionStatePending),
		Priority:        1,
		DraftName:       skillName,
		LifecycleStatus: "draft",
		Metadata:        metadata,
		CreatedAt:       time.Now().UTC(),
	}
	if err := uc.unifiedStore.Create(ctx, row); err != nil {
		return nil, err
	}
	return unifiedToLegacySuggestionPtr(uc.lg, &row), nil
}
