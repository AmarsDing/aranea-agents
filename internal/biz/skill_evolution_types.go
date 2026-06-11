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
// TODO(debt): DEV-04 — Unify with SkillEvolutionSuggestion into a single model.
// Current plan: SkillEvolutionSuggestion will be the canonical model;
// SkillProposal will be deprecated after migration.
// See also: skill_evolution_suggestion_types.go, skill_evolution_unified.go.
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
// TODO(debt): DEV-04 — These mapping functions are transitional and will be
// removed once SkillProposal is deprecated in favor of SkillEvolutionSuggestion.

// ProposalStatusToSuggestion maps a SkillProposalStatus to the equivalent
// EvolutionSuggestionStatus. The "registered" status maps to "applied" as
// both represent "action has been executed".
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
