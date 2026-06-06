package biz

import "context"

// HealthMetricsProvider provides recent health metrics for skill ranking.
// Defined in biz to avoid tools→biz dependency. The adapter implementation
// lives in the service layer, bridging SkillHealthAggregator to this interface
// and the tools layer's skillrecommend.HealthMetricsProvider.
type HealthMetricsProvider interface {
	// GetRecentSuccessRate returns the success rate (0-1) for a skill over
	// the last N days. Returns an error if the data source is unavailable.
	GetRecentSuccessRate(ctx context.Context, skillID string, days int) (float64, error)
	// GetRecentAvgDuration returns the average invocation duration in
	// milliseconds for a skill over the last N days.
	GetRecentAvgDuration(ctx context.Context, skillID string, days int) (float64, error)
}
