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
	return a.uc.ListProposals(ctx, agentID, status, 0, 0)
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

type skillsButlerAnalyticsAdapter struct {
	uc      *biz.ExperienceAnalyticsUsecase
	agentID string
}

func (a skillsButlerAnalyticsAdapter) AnalyzeToolWeights(ctx context.Context) ([]biz.ToolWeightReport, error) {
	if a.uc == nil {
		return nil, nil
	}
	analysis, err := a.uc.AnalyzeToolWeights(ctx, a.agentID, time.Now().AddDate(0, 0, -30))
	if err != nil {
		return nil, err
	}
	reports := make([]biz.ToolWeightReport, 0, len(analysis.Items))
	for _, it := range analysis.Items {
		reports = append(reports, biz.ToolWeightReport{
			ToolKey:        it.ToolKey,
			CallCount:      it.CallCount,
			SuccessRate:    it.SuccessRate,
			AvgDurationMS:  it.AvgDurationMS,
			WeightScore:    it.WeightScore,
			Recommendation: it.Recommendation,
		})
	}
	return reports, nil
}

func (a skillsButlerAnalyticsAdapter) AnalyzeSkillHealth(ctx context.Context) ([]biz.SkillHealth, error) {
	if a.uc == nil {
		return nil, nil
	}
	analysis, err := a.uc.AnalyzeSkillHealth(ctx, a.agentID, time.Now().AddDate(0, 0, -7))
	if err != nil {
		return nil, err
	}
	reports := make([]biz.SkillHealth, 0, len(analysis.Items))
	for _, it := range analysis.Items {
		trend := "stable"
		if it.InvokeCount == 0 {
			trend = "dormant"
		} else if it.SuccessRate < 0.5 {
			trend = "declining"
		} else if it.SuccessRate >= 0.9 {
			trend = "rising"
		}
		reports = append(reports, biz.SkillHealth{
			SkillID:        it.SkillID,
			InvokeCount7d:  it.InvokeCount,
			SuccessRate:    it.SuccessRate,
			AvgDurationMS:  it.AvgDurationMS,
			Trend:          trend,
			HealthStatus:   it.HealthStatus,
			Recommendation: it.Recommendation,
		})
	}
	return reports, nil
}

func (a skillsButlerAnalyticsAdapter) AnalyzeOrchestration(ctx context.Context, timeRange string, modeFilter string) ([]biz.OrchestrationModeReport, error) {
	if a.uc == nil {
		return nil, nil
	}
	since := bizTimeRangeToSince(timeRange)
	analysis, err := a.uc.AnalyzeOrchestration(ctx, a.agentID, since)
	if err != nil {
		return nil, err
	}
	reports := make([]biz.OrchestrationModeReport, 0, len(analysis.Items))
	for _, it := range analysis.Items {
		if modeFilter != "" && it.Mode != modeFilter {
			continue
		}
		reports = append(reports, biz.OrchestrationModeReport{
			Mode:        it.Mode,
			SuccessRate: it.SuccessRate,
			DQScore:     it.DQScore,
		})
	}
	return reports, nil
}

func bizTimeRangeToSince(tr string) time.Time {
	now := time.Now()
	switch tr {
	case "7d":
		return now.AddDate(0, 0, -7)
	case "30d":
		return now.AddDate(0, 0, -30)
	case "90d":
		return now.AddDate(0, 0, -90)
	default:
		return now.AddDate(0, 0, -30)
	}
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
