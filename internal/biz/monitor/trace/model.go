// Package trace implements the monitor trace domain: the flow-log → trace
// projector, the TRACE-01 file appender, and the trace persistence ports.
//
// DEV-05: split out of the monitor root package. Ports are narrow and live
// next to their consumer (port-follows-package); the data layer's monitorRepo
// satisfies them implicitly.
package trace

import "context"

// TraceWrite is the insert payload for a monitor_traces row.
type TraceWrite struct {
	TraceID       string
	SessionID     string
	RunID         string
	InvocationID  string
	AgentID       string
	Provider      string
	Model         string
	TeamID        string
	ParentTraceID string
	Name          string
	Status        string
	DurationMs    int64
	SpanCount     int
	ErrorCount    int
	TotalTokens   int64
	TotalCostUsd  float64
	MetadataJSON  string
}

// TraceSpanWrite is the upsert payload for a monitor_trace_spans row.
type TraceSpanWrite struct {
	TraceID        string
	SpanID         string
	ParentSpanID   string
	Kind           string
	Name           string
	StartedAt      int64
	EndedAt        int64
	Status         string
	AttributesJSON string
	ErrorJSON      string
}

// TraceSpan is the read model for one persisted span row (monitor_trace_spans).
// Timestamps are Unix milliseconds; EndedAt may be 0 for a still-open span.
type TraceSpan struct {
	SpanID         string
	ParentSpanID   string
	Kind           string
	Name           string
	StartedAt      int64
	EndedAt        int64
	Status         string
	AttributesJSON string
	ErrorJSON      string
}

// TraceCompletion carries the terminal-state fields written when a run
// completes (or is backfilled). Provider/Model are backfilled only when the
// stored column is still empty.
type TraceCompletion struct {
	Status       string
	DurationMs   int64
	SpanCount    int
	ErrorCount   int
	TotalTokens  int64
	TotalCostUsd float64
	Provider     string
	Model        string
}

// UsageAggregate holds token/cost aggregates for one trace, computed from
// model_token_usage_events (the authoritative cost source).
type UsageAggregate struct {
	TotalTokens  int64
	TotalCostUsd float64
	Provider     string
	Model        string
	CallCount    int
}

// Writer is the narrow trace-persistence port the Projector needs.
// Stability:evolving
type Writer interface {
	InsertMonitorTrace(ctx context.Context, tw TraceWrite) error
	UpsertMonitorTraceSpan(ctx context.Context, sw TraceSpanWrite) error
	UpdateMonitorTraceCompletion(ctx context.Context, traceID string, c TraceCompletion) error
	EnsureTraceSchema(ctx context.Context) error
}

// UsageRepo aggregates token usage events for a single trace.
// Stability:evolving
type UsageRepo interface {
	AggregateUsageByTrace(ctx context.Context, traceID string) (UsageAggregate, error)
}

// SpanReader reads persisted spans for a single trace, ordered by start time.
// Stability:evolving
type SpanReader interface {
	ListMonitorTraceSpans(ctx context.Context, traceID string) ([]TraceSpan, error)
}
