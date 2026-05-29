package graph

import (
	"sync"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// NodeExecutionSummary is a serializable per-node execution record for WS consumers.
type NodeExecutionSummary struct {
	NodeID     string `json:"node_id"`
	NodeType   string `json:"node_type"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	StepNumber int    `json:"step_number,omitempty"`
}

// ExecutionSummary aggregates a single graph run for graph_execution_done events.
type ExecutionSummary struct {
	ExecutionID   string                 `json:"execution_id"`
	GraphID       string                 `json:"graph_id"`
	TotalSteps    int                    `json:"total_steps"`
	DurationMS    int64                  `json:"duration_ms"`
	Nodes         []NodeExecutionSummary `json:"nodes"`
	FinalStateKeys int                   `json:"final_state_keys,omitempty"`
}

// ExecutionSummaryTracker records node completions until the graph finishes.
type ExecutionSummaryTracker struct {
	mu          sync.Mutex
	executionID string
	graphID     string
	nodes       []NodeExecutionSummary
}

func NewExecutionSummaryTracker(executionID, graphID string) *ExecutionSummaryTracker {
	return &ExecutionSummaryTracker{
		executionID: executionID,
		graphID:     graphID,
		nodes:       make([]NodeExecutionSummary, 0, 8),
	}
}

const (
	NodeExecStatusSuccess = "success"
	NodeExecStatusError  = "error"
)

func (t *ExecutionSummaryTracker) RecordNodeComplete(meta trpcgraph.NodeExecutionMetadata) {
	if t == nil {
		return
	}
	status := NodeExecStatusSuccess
	errMsg := meta.Error
	if errMsg != "" {
		status = NodeExecStatusError
	}
	durMS := meta.Duration.Milliseconds()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nodes = append(t.nodes, NodeExecutionSummary{
		NodeID:     meta.NodeID,
		NodeType:   string(meta.NodeType),
		Status:     status,
		DurationMS: durMS,
		Error:      errMsg,
		StepNumber: meta.StepNumber,
	})
}

func (t *ExecutionSummaryTracker) Snapshot(completion trpcgraph.CompletionMetadata) ExecutionSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	nodes := append([]NodeExecutionSummary(nil), t.nodes...)
	return ExecutionSummary{
		ExecutionID:    t.executionID,
		GraphID:        t.graphID,
		TotalSteps:     completion.TotalSteps,
		DurationMS:     completion.TotalDuration.Milliseconds(),
		Nodes:          nodes,
		FinalStateKeys: completion.FinalStateKeys,
	}
}
