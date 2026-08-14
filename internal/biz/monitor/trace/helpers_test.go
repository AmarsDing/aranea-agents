package trace_test

import (
	"context"

	"aranea-agents/internal/biz/monitor/trace"
	"aranea-agents/pkg/loggateway"
)

// mockTraceWriter is a function-field mock for trace.Writer.
type mockTraceWriter struct {
	insertMonitorTraceFn           func(ctx context.Context, tw trace.TraceWrite) error
	upsertMonitorTraceSpanFn       func(ctx context.Context, sw trace.TraceSpanWrite) error
	updateMonitorTraceCompletionFn func(ctx context.Context, traceID string, c trace.TraceCompletion) error
	ensureTraceSchemaFn            func(ctx context.Context) error
}

func (m *mockTraceWriter) InsertMonitorTrace(ctx context.Context, tw trace.TraceWrite) error {
	if m.insertMonitorTraceFn != nil {
		return m.insertMonitorTraceFn(ctx, tw)
	}
	return nil
}

func (m *mockTraceWriter) UpsertMonitorTraceSpan(ctx context.Context, sw trace.TraceSpanWrite) error {
	if m.upsertMonitorTraceSpanFn != nil {
		return m.upsertMonitorTraceSpanFn(ctx, sw)
	}
	return nil
}

func (m *mockTraceWriter) UpdateMonitorTraceCompletion(ctx context.Context, traceID string, c trace.TraceCompletion) error {
	if m.updateMonitorTraceCompletionFn != nil {
		return m.updateMonitorTraceCompletionFn(ctx, traceID, c)
	}
	return nil
}

func (m *mockTraceWriter) EnsureTraceSchema(ctx context.Context) error {
	if m.ensureTraceSchemaFn != nil {
		return m.ensureTraceSchemaFn(ctx)
	}
	return nil
}

// mockUsageRepo is a function-field mock for trace.UsageRepo.
type mockUsageRepo struct {
	aggregateFn func(ctx context.Context, traceID string) (trace.UsageAggregate, error)
	calls       []string
}

func (m *mockUsageRepo) AggregateUsageByTrace(ctx context.Context, traceID string) (trace.UsageAggregate, error) {
	m.calls = append(m.calls, traceID)
	if m.aggregateFn != nil {
		return m.aggregateFn(ctx, traceID)
	}
	return trace.UsageAggregate{}, nil
}

// warnCountingLogger counts Warn calls so tests can assert on log volume.
type warnCountingLogger struct {
	warnCount int
}

func (l *warnCountingLogger) Debug(string, ...loggateway.Field) {}
func (l *warnCountingLogger) Info(string, ...loggateway.Field)  {}
func (l *warnCountingLogger) Warn(string, ...loggateway.Field)  { l.warnCount++ }
func (l *warnCountingLogger) Error(string, ...loggateway.Field) {}
func (l *warnCountingLogger) With(...loggateway.Field) loggateway.Logger {
	return l
}
