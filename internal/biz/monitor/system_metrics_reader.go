package monitor

import (
	"context"
)

// MonitorSystemMetricsReader reads system metrics from the monitor Usecase.
// It adapts the Usecase's GetRunnerMetrics method to the SystemMetricsReader interface.
type MonitorSystemMetricsReader struct {
	uc *Usecase
}

// NewMonitorSystemMetricsReader creates a new SystemMetricsReader backed by the monitor Usecase.
func NewMonitorSystemMetricsReader(uc *Usecase) *MonitorSystemMetricsReader {
	return &MonitorSystemMetricsReader{uc: uc}
}

// ReadSystemMetrics reads current system metrics from the monitor usecase.
func (r *MonitorSystemMetricsReader) ReadSystemMetrics(ctx context.Context) (SystemMetrics, error) {
	if r == nil || r.uc == nil {
		return SystemMetrics{}, nil
	}

	metrics, err := r.uc.GetRunnerMetrics(ctx, 5) // 5-minute window
	if err != nil {
		return SystemMetrics{}, err
	}

	return SystemMetrics{
		ProviderLatencyMs: int64(metrics.P95DurationMs),
		MemoryUsagePct:    0, // Memory usage is not available from runner metrics; will be populated by external monitors
		SessionBacklog:    int(metrics.ErrorRuns),
	}, nil
}
