package session

import "context"

// SessionMetricsReader reads session metrics from the session_metrics table.
type SessionMetricsReader interface {
	GetSessionMetrics(ctx context.Context, sessionID string) (*SessionMetrics, error)
	ListSessionMetricsByIDs(ctx context.Context, ids []string) (map[string]*SessionMetrics, error)
}

// SessionMetricsWriter writes session metrics to the session_metrics table.
type SessionMetricsWriter interface {
	UpsertSessionMetrics(ctx context.Context, sessionID string, delta *SessionMetricsDelta) error
	ApplyMetricsDelta(ctx context.Context, d *SessionMetricsDelta) error
}

// SessionRuntimeReader reads session runtime state from the session_runtime table.
type SessionRuntimeReader interface {
	GetSessionRuntime(ctx context.Context, sessionID string) (*SessionRuntime, error)
}

// SessionRuntimeWriter writes session runtime state to the session_runtime table.
type SessionRuntimeWriter interface {
	UpsertSessionRuntime(ctx context.Context, sessionID string, runtime *SessionRuntime) error
	TransitionSessionStatus(ctx context.Context, sessionID string, currentStatus string, newStatus string, statusReason string, statusChangedAt string) error
}

// SessionMetrics holds the metrics fields for a session.
type SessionMetrics struct {
	SessionID            string
	MessageCount         int
	RunCount             int
	ModelCallCount       int
	ToolCallCount        int
	SkillCallCount       int
	MCPCallCount         int
	InputTokens          int
	OutputTokens         int
	TotalTokens          int
	TotalCostMicroUSD    int64
	AvgLatencyMs         float64
	ErrorCount           int
	ContextUsedTokens    int
	ContextUsedRatio     float64
	MaxContextUsedRatio  float64
	ContextStatus        string
	LastMessageAt        string
	UpdatedAt            string
}

// SessionRuntime holds the runtime state for a session.
type SessionRuntime struct {
	SessionID          string
	SessionRevision    int
	StateJSON          string
	RunnerSnapshotJSON string
	MetadataJSON       string
	CompressVersion    int
	UpdatedAt          string
}
