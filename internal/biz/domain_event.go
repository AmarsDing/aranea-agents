package biz

import (
	"time"
)

type DomainEventType string

const (
	DomainEventRunnerCompletion DomainEventType = "runner_completion"
	DomainEventGraphNodeStart   DomainEventType = "graph_node_start"
	DomainEventGraphNodeEnd     DomainEventType = "graph_node_end"
	DomainEventGraphNodeError   DomainEventType = "graph_node_error"
	DomainEventGraphInterrupt   DomainEventType = "graph_interrupt"
	// DomainEventGraphExecutionError is a graph-level fatal error (Pregel
	// error: panic / max steps / executeGraph failure) — N1.
	DomainEventGraphExecutionError DomainEventType = "graph_execution_error"
	// DomainEventGraphDone is the explicit framework completion event; it is
	// the ONLY signal allowed to converge an execution to Completed — N1.
	DomainEventGraphDone DomainEventType = "graph_done"
)

type DomainEvent struct {
	ID        string
	Type      DomainEventType
	Author    string
	SessionID string
	TeamID    string
	Timestamp time.Time

	RequestID        string
	InvocationID     string
	RunID            string
	TraceID          string
	AgentID          string
	AgentDisplayName string
	RunKind          string
	DurationMS       int64
	UsageEventID     string
	TurnStartedAt    time.Time

	Error *DomainError `json:",omitempty"`
	Usage *DomainUsage `json:",omitempty"`
}

type DomainError struct {
	Type    string
	Message string
}

type DomainUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
