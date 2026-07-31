package monitor

import "context"

// LogPair is a key-value pair for structured flow log extras.
type LogPair struct {
	Key   string
	Value any
}

// FlowLogWriter abstracts user-visible flow log (流程日志) emission so this
// package depends neither on internal/event nor on internal/biz (the biz root
// re-exports monitor types, so importing it here would create an import
// cycle). It mirrors biz.FlowLogWriter; the service layer adapts its
// ProvideFlowLogWriter output to this interface. Nil-safe: callers must
// nil-check before use (tests may pass nil).
type FlowLogWriter interface {
	LogFlowStart(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair)
	LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair)
	LogFlowError(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair)
}
