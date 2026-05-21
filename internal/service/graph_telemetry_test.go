package service

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/telemetry/turntrace"
)

func TestGraphExecutionTelemetryFinishOnce(t *testing.T) {
	tel := NewGraphExecutionTelemetry()
	ctx, bridge, _ := turntrace.Start(t.Context(), turntrace.Config{
		Domain:    turntrace.DomainGraph,
		SpanName:  "graph.execute",
		SessionID: "sess-1",
		RunID:     "exec-1",
	})
	_ = ctx
	tel.Bind("exec-1", bridge)
	tel.OnGraphExecutionComplete(&biz.GraphExecution{ID: "exec-1", Status: "completed"})
	tel.EnsureFinished("exec-1", nil)
}

func TestGraphExecutionTelemetryFailedExecution(t *testing.T) {
	tel := NewGraphExecutionTelemetry()
	_, bridge, _ := turntrace.Start(t.Context(), turntrace.Config{
		Domain:   turntrace.DomainGraph,
		SpanName: "graph.execute",
		RunID:    "exec-2",
	})
	tel.Bind("exec-2", bridge)
	tel.OnGraphExecutionComplete(&biz.GraphExecution{
		ID:           "exec-2",
		Status:       "failed",
		ErrorMessage: "build failed",
	})
}
