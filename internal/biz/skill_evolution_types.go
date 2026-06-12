package biz

import "time"

type SkillProposalStatus string

const (
	SkillProposalStatusPending    SkillProposalStatus = "pending"
	SkillProposalStatusApproved   SkillProposalStatus = "approved"
	SkillProposalStatusRejected   SkillProposalStatus = "rejected"
	SkillProposalStatusRegistered SkillProposalStatus = "registered"
)

// SkillProposal represents an evolution proposal from the SkillEvolutionLoop,
// used for creating NEW skills from detected patterns.
//
// Deprecated: Use UnifiedEvolutionSuggestion with ActionType=create_skill instead.
// SkillProposal will be removed in a future release once all callers are migrated.
// Migration: UnifiedEvolutionSuggestion{TargetType: "agent", ActionType: "create_skill", TargetID: proposal.AgentID, DraftBody: proposal.SkillMD, DraftName: proposal.SkillName}
type SkillProposal struct {
	ID          string
	AgentID     string
	PatternHash string
	PatternDesc string
	SkillName   string
	SkillMD     string
	Status      SkillProposalStatus
	ApprovedBy  string
	RejectedBy  string
	CreatedAt   time.Time
	ApprovedAt  *time.Time
}

type ToolCallRecord struct {
	ToolName  string
	Arguments string
	Result    string
	Success   bool
	CalledAt  time.Time
}

// ── Status mapping between SkillProposal and SkillEvolutionSuggestion ────────
//
// Deprecated: These mapping functions are transitional and will be removed
// once SkillProposal is fully deprecated. Use UnifiedEvolutionSuggestion
// directly instead of converting between legacy types.

// ProposalStatusToSuggestion maps a SkillProposalStatus to the equivalent
// EvolutionSuggestionStatus. The "registered" status maps to "applied" as
// both represent "action has been executed".
//
// Deprecated: Use UnifiedEvolutionSuggestion.Status (string) directly.
func ProposalStatusToSuggestion(s SkillProposalStatus) EvolutionSuggestionStatus {
	switch s {
	case SkillProposalStatusPending:
		return EvoSuggestionPending
	case SkillProposalStatusApproved:
		return EvoSuggestionApproved
	case SkillProposalStatusRejected:
		return EvoSuggestionRejected
	case SkillProposalStatusRegistered:
		return EvoSuggestionApplied
	default:
		return EvoSuggestionPending
	}
}

// SuggestionStatusToProposal maps an EvolutionSuggestionStatus to the equivalent
// SkillProposalStatus. The "applied" status maps to "registered" as both
// represent "action has been executed".
//
// Deprecated: Use UnifiedEvolutionSuggestion.Status (string) directly.
func SuggestionStatusToProposal(s EvolutionSuggestionStatus) SkillProposalStatus {
	switch s {
	case EvoSuggestionPending:
		return SkillProposalStatusPending
	case EvoSuggestionApproved:
		return SkillProposalStatusApproved
	case EvoSuggestionRejected:
		return SkillProposalStatusRejected
	case EvoSuggestionApplied:
		return SkillProposalStatusRegistered
	default:
		return SkillProposalStatusPending
	}
}
