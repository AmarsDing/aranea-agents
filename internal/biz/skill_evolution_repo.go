package biz

import "context"

type SkillProposalReader interface {
	ListByAgent(ctx context.Context, agentID string, status string, limit int, offset int) ([]SkillProposal, error)
	GetByID(ctx context.Context, id string) (SkillProposal, error)
	GetByPatternHash(ctx context.Context, agentID string, hash string) (*SkillProposal, error)
}

type SkillProposalWriter interface {
	Create(ctx context.Context, p SkillProposal) (SkillProposal, error)
	UpdateStatus(ctx context.Context, id string, status SkillProposalStatus, operator string) (SkillProposal, error)
}

type SkillProposalReadWriter interface {
	SkillProposalReader
	SkillProposalWriter
}
