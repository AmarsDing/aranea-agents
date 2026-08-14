package trace_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor/trace"
	"aranea-agents/pkg/loggateway"
)

func newTestTraceProjectorWithUsage(repo trace.Writer, usage trace.UsageRepo) *trace.TraceProjector {
	return trace.NewTraceProjector(repo, loggateway.NewNoop(), usage, &stubMonitorBus{})
}

// When usage events exist for the trace, completion must backfill tokens/cost
// from the usage aggregate (the authoritative cost source) and provider/model
// when the in-memory trace lacks them.
func TestTraceProjector_OnRunnerCompletion_BackfillsUsageAggregate(t *testing.T) {
	var got trace.TraceCompletion
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(_ context.Context, traceID string, c trace.TraceCompletion) error {
			if traceID != "trace-agg" {
				t.Errorf("traceID = %q, want %q", traceID, "trace-agg")
			}
			got = c
			return nil
		},
	}
	usage := &mockUsageRepo{
		aggregateFn: func(_ context.Context, traceID string) (trace.UsageAggregate, error) {
			if traceID != "trace-agg" {
				t.Errorf("aggregate traceID = %q, want %q", traceID, "trace-agg")
			}
			return trace.UsageAggregate{
				TotalTokens:  1234,
				TotalCostUsd: 0.0567,
				Provider:     "openai",
				Model:        "gpt-4o",
				CallCount:    2,
			}, nil
		},
	}
	p := newTestTraceProjectorWithUsage(repo, usage)
	p.AddTestTrace("trace-agg", time.Now())

	p.OnRunnerCompletion(context.Background(), "trace-agg", "ok", 800)

	if len(usage.calls) != 1 {
		t.Fatalf("AggregateUsageByTrace calls = %d, want 1", len(usage.calls))
	}
	if got.TotalTokens != 1234 {
		t.Errorf("TotalTokens = %d, want 1234", got.TotalTokens)
	}
	if got.TotalCostUsd != 0.0567 {
		t.Errorf("TotalCostUsd = %v, want 0.0567", got.TotalCostUsd)
	}
	if got.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", got.Provider, "openai")
	}
	if got.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", got.Model, "gpt-4o")
	}
	if got.Status != "ok" {
		t.Errorf("Status = %q, want %q", got.Status, "ok")
	}
}

// When no usage events exist (CallCount == 0), completion keeps the in-memory
// token count and leaves provider/model empty for the repo-side COALESCE.
func TestTraceProjector_OnRunnerCompletion_EmptyUsageKeepsInMemory(t *testing.T) {
	var got trace.TraceCompletion
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(_ context.Context, _ string, c trace.TraceCompletion) error {
			got = c
			return nil
		},
	}
	usage := &mockUsageRepo{} // zero aggregate
	p := newTestTraceProjectorWithUsage(repo, usage)
	p.AddTestTrace("trace-empty", time.Now())

	p.OnRunnerCompletion(context.Background(), "trace-empty", "ok", 300)

	if got.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0", got.TotalTokens)
	}
	if got.Provider != "" || got.Model != "" {
		t.Errorf("Provider/Model = %q/%q, want empty", got.Provider, got.Model)
	}
}

// Aggregate failure must not block completion — the trace still closes with
// in-memory values.
func TestTraceProjector_OnRunnerCompletion_AggregateErrorTolerated(t *testing.T) {
	called := false
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(_ context.Context, _ string, c trace.TraceCompletion) error {
			called = true
			if c.Status != "ok" {
				t.Errorf("Status = %q, want ok", c.Status)
			}
			return nil
		},
	}
	usage := &mockUsageRepo{
		aggregateFn: func(context.Context, string) (trace.UsageAggregate, error) {
			return trace.UsageAggregate{}, fmt.Errorf("db down")
		},
	}
	p := newTestTraceProjectorWithUsage(repo, usage)
	p.AddTestTrace("trace-err", time.Now())

	p.OnRunnerCompletion(context.Background(), "trace-err", "ok", 100)
	if !called {
		t.Error("UpdateMonitorTraceCompletion not called after aggregate error")
	}
}

// Nil usage repo keeps the legacy behaviour (no aggregation).
func TestTraceProjector_OnRunnerCompletion_NilUsageRepo(t *testing.T) {
	called := false
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(_ context.Context, _ string, _ trace.TraceCompletion) error {
			called = true
			return nil
		},
	}
	p := newTestTraceProjectorWithUsage(repo, nil)
	p.AddTestTrace("trace-nil", time.Now())
	p.OnRunnerCompletion(context.Background(), "trace-nil", "ok", 100)
	if !called {
		t.Error("UpdateMonitorTraceCompletion not called with nil usage repo")
	}
}
