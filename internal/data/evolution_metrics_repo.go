package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
)

type evolutionMetricsRepo struct {
	data *Data
}

func NewEvolutionMetricsRepo(data *Data) biz.EvolutionMetricsRepo {
	return &evolutionMetricsRepo{data: data}
}

func (r *evolutionMetricsRepo) GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []biz.MetricDataPoint, error) {
	var total, success int64
	rows, err := r.data.entClient.ToolInvocation.Query().
		Where(
			toolinvocation.AgentIDEQ(agentID),
			toolinvocation.CreatedAtGTE(since.Format(time.RFC3339)),
		).
		All(ctx)
	if err != nil {
		return 0, nil, err
	}
	total = int64(len(rows))
	for _, row := range rows {
		if row.Status == "success" {
			success++
		}
	}
	rate := 0.0
	if total > 0 {
		rate = float64(success) / float64(total)
	}
	return rate, nil, nil
}

func (r *evolutionMetricsRepo) GetRetrievalQuality(ctx context.Context, agentID string, since time.Time) (float64, []biz.MetricDataPoint, error) {
	return 0, nil, nil
}

func (r *evolutionMetricsRepo) GetEpisodeCount(ctx context.Context, agentID string, since time.Time) (int, error) {
	count, err := r.data.entClient.Session.Query().
		Where(
			session.AgentIDEQ(agentID),
			session.CreatedAtGTE(since.Format(time.RFC3339)),
		).
		Count(ctx)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *evolutionMetricsRepo) GetNegativeFeedbackCount(ctx context.Context, agentID string, since time.Time) (int, error) {
	return 0, nil
}
