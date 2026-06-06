package biz

import (
	"context"
	"time"

	"aranea-agents/internal/tools/skillrecommend"
)

// SkillHealthMetricsAdapter adapts the Biz layer's SkillHealthAggregator
// to the Tools layer's HealthMetricsProvider interface. This avoids a direct
// dependency from Tools → Biz.
type SkillHealthMetricsAdapter struct {
	agg SkillHealthAggregator
}

// NewSkillHealthMetricsAdapter creates a new adapter.
func NewSkillHealthMetricsAdapter(agg SkillHealthAggregator) *SkillHealthMetricsAdapter {
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

// Compile-time check that the adapter satisfies the interface.
var _ skillrecommend.HealthMetricsProvider = (*SkillHealthMetricsAdapter)(nil)
