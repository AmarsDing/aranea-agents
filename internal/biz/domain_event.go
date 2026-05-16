package biz

import "time"

type DomainEventType string

const (
	DomainEventRunnerCompletion DomainEventType = "runner_completion"
	DomainEventStateDelta       DomainEventType = "state_delta"
	DomainEventError            DomainEventType = "error"
	DomainEventGraphNodeStart   DomainEventType = "graph_node_start"
	DomainEventGraphNodeEnd     DomainEventType = "graph_node_end"
	DomainEventGraphNodeError   DomainEventType = "graph_node_error"
	DomainEventGraphInterrupt   DomainEventType = "graph_interrupt"
	DomainEventTextDelta        DomainEventType = "text_delta"
	DomainEventToolCall         DomainEventType = "tool_call"
	DomainEventToolResult       DomainEventType = "tool_result"
)

type DomainEvent struct {
	ID        string
	Type      DomainEventType
	Author    string
	SessionID string
	TeamID    string
	Timestamp time.Time

	Content    *DomainContent    `json:",omitempty"`
	StateDelta *DomainStateDelta `json:",omitempty"`
	Error      *DomainError      `json:",omitempty"`
	Usage      *DomainUsage      `json:",omitempty"`
	ToolCall   *DomainToolCall   `json:",omitempty"`
	GraphNode  *DomainGraphNode  `json:",omitempty"`
}

type DomainContent struct {
	Text      string
	Reasoning string
	IsPartial bool
}

type DomainStateDelta struct {
	Operation string
	Path      string
	ValueJSON string
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

type DomainToolCall struct {
	ID            string
	Name          string
	ArgumentsJSON string
	ResultJSON    string
	Status        string
	DurationMS    int64
}

type DomainGraphNode struct {
	NodeID string
	Error  string
}

type DomainEventPublisher interface {
	PublishDomainEvent(event DomainEvent)
}

type DomainEventSubscriber interface {
	SubscribeDomainEvents() (<-chan DomainEvent, func())
}
