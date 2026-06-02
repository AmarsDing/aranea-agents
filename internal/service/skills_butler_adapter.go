package service

import (
	"context"
	"errors"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/skills_butler"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type skillsButlerSkillUsecaseAdapter struct {
	uc *biz.SkillEvolutionUsecase
}

func (a skillsButlerSkillUsecaseAdapter) ListProposals(ctx context.Context, agentID string, status string) ([]biz.SkillProposal, error) {
	if a.uc == nil {
		return nil, nil
	}
	return a.uc.ListProposals(ctx, agentID, status)
}

func (a skillsButlerSkillUsecaseAdapter) ApproveProposal(ctx context.Context, id string, approvedBy string) (biz.SkillProposal, error) {
	if a.uc == nil {
		return biz.SkillProposal{}, nil
	}
	return a.uc.ApproveProposal(ctx, id, approvedBy)
}

func (a skillsButlerSkillUsecaseAdapter) RejectProposal(ctx context.Context, id string, rejectedBy string) (biz.SkillProposal, error) {
	if a.uc == nil {
		return biz.SkillProposal{}, nil
	}
	return a.uc.RejectProposal(ctx, id, rejectedBy)
}

func (a skillsButlerSkillUsecaseAdapter) RegisterApproved(ctx context.Context, id string) (biz.SkillProposal, error) {
	if a.uc == nil {
		return biz.SkillProposal{}, nil
	}
	return a.uc.RegisterApproved(ctx, id)
}

func (a skillsButlerSkillUsecaseAdapter) CreateProposal(ctx context.Context, proposal biz.SkillProposal) (biz.SkillProposal, error) {
	if a.uc == nil {
		return biz.SkillProposal{}, nil
	}
	return a.uc.CreateProposal(ctx, proposal)
}

type skillsButlerEvolutionAdapter struct {
	uc *biz.EvolutionUsecase
}

func (a skillsButlerEvolutionAdapter) GetEvolutionMetrics(ctx context.Context, agentID string, timeRange string) (biz.EvolutionMetrics, error) {
	if a.uc == nil {
		return biz.EvolutionMetrics{}, nil
	}
	return a.uc.GetEvolutionMetrics(ctx, agentID, timeRange)
}

type skillsButlerQueryAdapter struct {
	reader biz.SkillInvocationStatsReader
}

func (a skillsButlerQueryAdapter) GetSkillInvocationStats(ctx context.Context, agentID string, since time.Time) ([]skills_butler.SkillInvocationStat, error) {
	if a.reader == nil {
		return nil, nil
	}
	stats, err := a.reader.GetSkillInvocationStats(ctx, agentID, since)
	if err != nil {
		return nil, err
	}
	out := make([]skills_butler.SkillInvocationStat, 0, len(stats))
	for _, s := range stats {
		out = append(out, skills_butler.SkillInvocationStat{
			SkillName:     s.SkillName,
			Count:         s.Count,
			SuccessRate:   s.SuccessRate,
			AvgDurationMs: s.AvgDurationMs,
		})
	}
	return out, nil
}

type skillsButlerRegistrationAdapter struct {
	uc *biz.SkillUsecase
}

func NewSkillsButlerRegistrationAdapter(uc *biz.SkillUsecase) biz.SkillRegistrationPort {
	return skillsButlerRegistrationAdapter{uc: uc}
}

func (a skillsButlerRegistrationAdapter) RegisterSkill(ctx context.Context, agentID string, name string, skillMD string) error {
	if a.uc == nil {
		return nil
	}
	_, err := a.uc.Create(ctx, biz.SkillCreateInput{
		Name: name,
		Slug: name,
		Body: skillMD,
	})
	return err
}

func (a skillsButlerRegistrationAdapter) SkillExists(ctx context.Context, agentID string, name string) (bool, error) {
	if a.uc == nil {
		return false, nil
	}
	_, err := a.uc.GetBySlug(ctx, name)
	if err != nil {
		if errors.Is(err, biz.ErrNotFound) || kerrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
