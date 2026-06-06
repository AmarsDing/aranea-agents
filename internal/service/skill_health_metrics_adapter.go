package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/skillrecommend"
)

// SkillHealthMetricsAdapter adapts the Biz layer's SkillHealthAggregator
// to both biz.HealthMetricsProvider and skillrecommend.HealthMetricsProvider.
// Placed in the service layer because it depends on both biz and tools packages.
type SkillHealthMetricsAdapter struct {
	agg biz.SkillHealthAggregator
}

// NewSkillHealthMetricsAdapter creates a new adapter.
func NewSkillHealthMetricsAdapter(agg biz.SkillHealthAggregator) *SkillHealthMetricsAdapter {
	return &SkillHealthMetricsAdapter{agg: agg}
}

// GetRecentSuccessRate returns the success rate (0-1) for a skill over the
// last N days by delegating to SkillHealthAggregator.GetHealthMetrics.
func (a *SkillHealthMetricsAdapter) GetRecentSuccessRate(ctx context.Context, skillID string, days int) (float64, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	metrics, err := a.agg.GetHealthMetrics(ctx, skillID, since)
	if err != nil {
		return 0, err
	}
	if metrics == nil {
		return 0, nil
	}
	return metrics.SuccessRate, nil
}

// GetRecentAvgDuration returns the average duration in milliseconds for a
// skill over the last N days by delegating to SkillHealthAggregator.GetHealthMetrics.
func (a *SkillHealthMetricsAdapter) GetRecentAvgDuration(ctx context.Context, skillID string, days int) (float64, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	metrics, err := a.agg.GetHealthMetrics(ctx, skillID, since)
	if err != nil {
		return 0, err
	}
	if metrics == nil {
		return 0, nil
	}
	return metrics.AvgDurationMS, nil
}

// Compile-time checks that the adapter satisfies both interfaces.
var _ biz.HealthMetricsProvider = (*SkillHealthMetricsAdapter)(nil)
var _ skillrecommend.HealthMetricsProvider = (*SkillHealthMetricsAdapter)(nil)
