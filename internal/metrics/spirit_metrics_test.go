package metrics

import (
	"testing"
)

// TestSpiritMetrics_Registered verifies that the Wave 1 P3-3 Spirit orchestration
// metrics are registered and can observe values without panic. These metrics
// cover Plan/Allocate/Orchestrate phase durations, AgentFactory creation count,
// and Graph replan totals by type.
//
// Pattern follows callback_test.go: verify observation does not panic. Value
// assertions are avoided to keep the test dependency-free (no testutil import).
// Bucket configuration is verified by code review against the design doc
// (SpiritOrchDuration must include >=3600s for 24h long-task phases).
func TestSpiritMetrics_Registered(t *testing.T) {
	// Histogram observations — should not panic.
	SpiritPlanDuration.Observe(1.2)
	SpiritAllocDuration.Observe(0.8)
	SpiritOrchDuration.Observe(45.0)

	// Counter increments — should not panic.
	AgentFactoryCreated.Inc()
	GraphReplanTotal.WithLabelValues("retry").Inc()
	GraphReplanTotal.WithLabelValues("reroute").Inc()
	GraphReplanTotal.WithLabelValues("insert_fallback").Inc()
	GraphReplanTotal.WithLabelValues("rebuild_subgraph").Inc()
}
