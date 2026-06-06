package skillrecommend

import "context"

// HealthMetricsProvider bridges health metrics from the Biz layer into the
// Tools layer without creating a direct dependency. The Biz layer provides
// the concrete adapter; the Tools layer only depends on this interface.
type HealthMetricsProvider interface {
	// GetRecentSuccessRate returns the success rate (0-1) for a skill over
	// the last N days. Returns an error if the data source is unavailable.
	GetRecentSuccessRate(ctx context.Context, skillID string, days int) (float64, error)
	// GetRecentAvgDuration returns the average invocation duration in
	// milliseconds for a skill over the last N days.
	GetRecentAvgDuration(ctx context.Context, skillID string, days int) (float64, error)
}
