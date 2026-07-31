package monitor

import (
	"context"
	"time"
)

// CanaryFailureReader is a narrow port for reading the memory canary's
// consecutive failure streak. Implemented by *biz.MemoryCanaryStatus;
// defined here so biz/monitor does not depend on the biz root package
// (dependency direction: biz/monitor is inner).
type CanaryFailureReader interface {
	ConsecutiveFailures() int64
}

// MemoryCanaryMetric exposes the memory closed-loop canary failure streak to
// the alert engine (P0 canary). A value >= 1 means the write → recall →
// archive loop is broken: memories are being written but cannot be recalled
// (or cannot be invalidated), i.e. the memory system is silently dead.
type MemoryCanaryMetric struct {
	reader CanaryFailureReader
}

func NewMemoryCanaryMetric(r CanaryFailureReader) *MemoryCanaryMetric {
	return &MemoryCanaryMetric{reader: r}
}

func (m *MemoryCanaryMetric) Key() string { return "memory.canary_consecutive_failures" }
func (m *MemoryCanaryMetric) Description() string {
	return "Consecutive memory canary failures (write → recall → archive loop)"
}
func (m *MemoryCanaryMetric) Catalog() AlertMetricInfo {
	return AlertMetricInfo{
		Key:                  m.Key(),
		Name:                 "Memory canary failures",
		Description:          "Consecutive failures of the memory closed-loop canary (fact write → recall with minScore → invalidate). Any value >= 1 means the memory pipeline is broken end-to-end.",
		Unit:                 "count",
		DefaultWindowMinutes: 5,
		SuggestedThreshold:   1,
	}
}
func (m *MemoryCanaryMetric) Evaluate(_ context.Context, _ time.Duration) (float64, error) {
	if m == nil || m.reader == nil {
		return 0, nil
	}
	return float64(m.reader.ConsecutiveFailures()), nil
}
