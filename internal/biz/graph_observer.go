package biz

// GraphExecutionObserver is notified when a graph execution finishes or aborts early.
// Implementations live in internal/service (OTel, metrics, etc.).
type GraphExecutionObserver interface {
	OnGraphExecutionComplete(exec *GraphExecution)
}
