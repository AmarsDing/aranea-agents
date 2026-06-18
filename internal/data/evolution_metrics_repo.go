package data

import (
	"context"
	"sort"
	"time"

	"aranea-agents/internal/biz"
	entsession "aranea-agents/internal/data/ent/session"
	toolinvocationpkg "aranea-agents/internal/data/ent/toolinvocation"
)

type evolutionMetricsRepo struct {
	data *Data
}

var _ biz.EvolutionMetricsRepo = (*evolutionMetricsRepo)(nil)

func NewEvolutionMetricsRepo(data *Data) biz.EvolutionMetricsRepo {
	return &evolutionMetricsRepo{data: data}
}

func (r *evolutionMetricsRepo) GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []biz.MetricDataPoint, error) {
	rows, err := r.data.RW().Read(ctx).ToolInvocation.Query().
		Where(
			toolinvocationpkg.AgentIDEQ(agentID),
			toolinvocationpkg.CreatedAtGTE(since.Format(time.RFC3339)),
		).
		All(ctx)
	if err != nil {
		return 0, nil, entErrToBizErr(err, "EVOLUTION_METRICS")
	}
	var total, success int64
	dayBuckets := make(map[string]struct{ total, success int64 })
	for _, row := range rows {
		total++
		if row.Status == "success" {
			success++
		}
		day := dateFromCreatedAt(row.CreatedAt)
		b := dayBuckets[day]
		b.total++
		if row.Status == "success" {
			b.success++
		}
		dayBuckets[day] = b
	}
	rate := 0.0
	if total > 0 {
		rate = float64(success) / float64(total)
	}
	series := buildSeries(dayBuckets, since)
	return rate, series, nil
}

func (r *evolutionMetricsRepo) GetRetrievalQuality(ctx context.Context, agentID string, since time.Time) (float64, []biz.MetricDataPoint, error) {
	memoryToolKeys := []string{"memory_search", "memory_load", "memory_recall"}
	rows, err := r.data.RW().Read(ctx).ToolInvocation.Query().
		Where(
			toolinvocationpkg.AgentIDEQ(agentID),
			toolinvocationpkg.CreatedAtGTE(since.Format(time.RFC3339)),
			toolinvocationpkg.ToolKeyIn(memoryToolKeys...),
		).
		All(ctx)
	if err != nil {
		return 0, nil, entErrToBizErr(err, "EVOLUTION_METRICS")
	}
	var total, success int64
	dayBuckets := make(map[string]struct{ total, success int64 })
	for _, row := range rows {
		total++
		if row.Status == "success" {
			success++
		}
		day := dateFromCreatedAt(row.CreatedAt)
		b := dayBuckets[day]
		b.total++
		if row.Status == "success" {
			b.success++
		}
		dayBuckets[day] = b
	}
	rate := 0.0
	if total > 0 {
		rate = float64(success) / float64(total)
	}
	series := buildSeries(dayBuckets, since)
	return rate, series, nil
}

func (r *evolutionMetricsRepo) GetEpisodeCount(ctx context.Context, agentID string, since time.Time) (int, error) {
	count, err := r.data.RW().Read(ctx).Session.Query().
		Where(
			entsession.AgentIDEQ(agentID),
			entsession.CreatedAtGTE(since.Format(time.RFC3339)),
		).
		Count(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "EVOLUTION_METRICS")
	}
	return count, nil
}

func (r *evolutionMetricsRepo) GetNegativeFeedbackCount(ctx context.Context, agentID string, since time.Time) (int, error) {
	count, err := r.data.RW().Read(ctx).ToolInvocation.Query().
		Where(
			toolinvocationpkg.AgentIDEQ(agentID),
			toolinvocationpkg.CreatedAtGTE(since.Format(time.RFC3339)),
			toolinvocationpkg.StatusNEQ("success"),
		).
		Count(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "EVOLUTION_METRICS")
	}
	return count, nil
}

func dateFromCreatedAt(createdAt string) string {
	if len(createdAt) >= 10 {
		return createdAt[:10]
	}
	return createdAt
}

func buildSeries(dayBuckets map[string]struct{ total, success int64 }, since time.Time) []biz.MetricDataPoint {
	days := make([]string, 0, len(dayBuckets))
	for d := range dayBuckets {
		days = append(days, d)
	}
	sort.Strings(days)
	series := make([]biz.MetricDataPoint, 0, len(days))
	for _, d := range days {
		b := dayBuckets[d]
		var v float64
		if b.total > 0 {
			v = float64(b.success) / float64(b.total)
		}
		series = append(series, biz.MetricDataPoint{Date: d, Value: v})
	}
	return series
}
