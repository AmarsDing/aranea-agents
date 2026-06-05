package biz

import "time"

type SkillProposalStatus string

const (
	SkillProposalStatusPending    SkillProposalStatus = "pending"
	SkillProposalStatusApproved   SkillProposalStatus = "approved"
	SkillProposalStatusRejected   SkillProposalStatus = "rejected"
	SkillProposalStatusRegistered SkillProposalStatus = "registered"
)

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
