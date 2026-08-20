package biz

import "time"

type SkillProposalStatus string

const (
	SkillProposalStatusPending    SkillProposalStatus = "pending"
	SkillProposalStatusApproved   SkillProposalStatus = "approved"
	SkillProposalStatusRejected   SkillProposalStatus = "rejected"
	SkillProposalStatusRegistered SkillProposalStatus = "registered"
)

// SkillProposal represents an evolution proposal from the skill evolution
// pipeline, used for creating NEW skills from detected patterns.
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
