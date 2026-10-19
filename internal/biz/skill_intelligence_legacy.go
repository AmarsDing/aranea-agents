package biz

import (
	"encoding/json"
	"time"

	"aranea-agents/pkg/loggateway"
)

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
func unifiedToLegacySuggestionPtr(lg loggateway.Logger, u *UnifiedEvolutionSuggestion) *SkillEvolutionSuggestion {
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
		// 审计 T2：metadata 损坏不再静默吞错，warn 后降级为空列表。
		if err := json.Unmarshal(raw, &sourceReportIDs); err != nil {
			lg.Warn("unifiedToLegacySuggestionPtr: corrupt source_report_ids metadata, degrading to empty",
				loggateway.StepID("skill_intelligence.bridge"),
				loggateway.Str("suggestion_id", u.ID),
				loggateway.Err(err))
		}
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

		TargetType: string(u.TargetType),
		TargetID:   u.TargetID,
		DraftName:  u.DraftName,
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
	case "applied", "registered":

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
