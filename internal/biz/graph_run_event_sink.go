package biz

import "context"

// GraphRunEventSink receives per-run lifecycle callbacks from
// GraphExecutionUsecase so external bridges (e.g. the twinmonitor OpenAPI
// compat facade) can stream run/node events to subscribers without
// modifying the execution core.
//
// Implementations must be non-blocking and nil-safe to register; the
// usecase invokes hooks synchronously on its event loop, so sinks should
// dispatch asynchronously (see WebhookDispatcher / safego patterns).
//
// Registration follows the SetTaskCoordinator pattern: optional, set once
// during wire assembly in the service layer.
type GraphRunEventSink interface {
	// OnNodeStarted fires when a graph node begins execution.
	// step is the runtime step number (monotonic per run).
	OnNodeStarted(ctx context.Context, execID, graphID, nodeID string, step int)
	// OnNodeCompleted fires when a graph node ends, with status
	// "completed" or "failed" (errMsg set on failure).
	OnNodeCompleted(ctx context.Context, execID, graphID, nodeID string, step int, status, errMsg string)
	// OnRunWaitingApproval fires when the run interrupts at a node
	// (human-in-the-loop approval point).
	OnRunWaitingApproval(ctx context.Context, execID, graphID, nodeID string)
	// OnRunCompleted fires when the run reaches a successful terminal state.
	OnRunCompleted(ctx context.Context, execID, graphID string, durationMs int64)
	// OnRunFailed fires when the run reaches a failed terminal state.
	OnRunFailed(ctx context.Context, execID, graphID, errMsg string)
	// OnRunCancelled fires when the run is cancelled by caller request.
	OnRunCancelled(ctx context.Context, execID, graphID string)
}
