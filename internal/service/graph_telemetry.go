package service

import (
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/telemetry/turntrace"
)

// GraphExecutionTelemetry binds OTel bridges to graph executions and finishes spans on completion.
type GraphExecutionTelemetry struct {
	mu      sync.Mutex
	bridges map[string]*turntrace.Bridge
}

var _ biz.GraphExecutionObserver = (*GraphExecutionTelemetry)(nil)

func NewGraphExecutionTelemetry() *GraphExecutionTelemetry {
	return &GraphExecutionTelemetry{bridges: make(map[string]*turntrace.Bridge)}
}

// Bind associates an OTel bridge with an execution id before ExecuteGraph runs.
func (t *GraphExecutionTelemetry) Bind(execID string, bridge *turntrace.Bridge) {
	if t == nil || execID == "" || bridge == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bridges[execID] = bridge
}

func (t *GraphExecutionTelemetry) OnGraphExecutionComplete(exec *biz.GraphExecution) {
	if t == nil || exec == nil {
		return
	}
	t.mu.Lock()
	bridge := t.bridges[exec.ID]
	if bridge != nil {
		delete(t.bridges, exec.ID)
	}
	t.mu.Unlock()
	if bridge != nil {
		bridge.Finish(graphExecutionFinishErr(exec))
	}
}

// EnsureFinished closes a bridge if it is still bound (unexpected early return).
func (t *GraphExecutionTelemetry) EnsureFinished(execID string, err error) {
	if t == nil || execID == "" {
		return
	}
	t.mu.Lock()
	bridge := t.bridges[execID]
	if bridge != nil {
		delete(t.bridges, execID)
	}
	t.mu.Unlock()
	if bridge != nil {
		bridge.Finish(err)
	}
}
