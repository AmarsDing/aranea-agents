package biz

import (
	"context"
	"time"
)

// ActivityKind classifies the semantic type of an Activity.
// Aligned with TaskBoardNodeKind for zero-inference frontend consumption.
type ActivityKind string

const (
	ActivityKindTask        ActivityKind = "task"         // Task description (user/agent perspective)
	ActivityKindThinking    ActivityKind = "thinking"     // Reasoning content
	ActivityKindAction      ActivityKind = "action"       // Tool invocation
	ActivityKindReply       ActivityKind = "reply"        // Agent reply (including final answer)
	ActivityKindSubTaskBoard ActivityKind = "sub_task_board" // Sub-task board (recursive nesting)
	ActivityKindEnd         ActivityKind = "end"          // Task completion marker
	ActivityKindError       ActivityKind = "error"        // Error information
	ActivityKindDelegate    ActivityKind = "delegate"     // Spirit delegates to team
	ActivityKindNotice      ActivityKind = "notice"       // System notification (context loading, status change)
)

// ActivityStatus represents the lifecycle status of an Activity.
type ActivityStatus string

const (
	ActivityStatusPending       ActivityStatus = "pending"
	ActivityStatusRunning       ActivityStatus = "running"
	ActivityStatusToolRunning   ActivityStatus = "tool_running"
	ActivityStatusToolBlocked   ActivityStatus = "tool_blocked"
	ActivityStatusCompleted     ActivityStatus = "completed"
	ActivityStatusFailed        ActivityStatus = "failed"
	ActivityStatusPartialFailure ActivityStatus = "partial_failure"
	ActivityStatusCancelled     ActivityStatus = "cancelled"
	ActivityStatusInterrupted   ActivityStatus = "interrupted"
)

// Activity is the domain model for a projected semantic unit.
// The backend projects runtime events into Activity records and pushes
// them to the frontend via WS, eliminating the need for frontend inference.
type Activity struct {
	ID               string
	Kind             ActivityKind
	Status           ActivityStatus
	SessionID        string
	TurnID           string
	ParentActivityID string
	Timestamp        time.Time
	DurationMs       int64

	// Token usage (kind=task, root Activity only)
	PromptTokens     int64
	CompletionTokens int64

	// Content fields (by kind)
	Content   string // task/reply/notice/end/error text
	Reasoning string // thinking reasoning content

	// Tool fields (kind=action)
	ToolName       string
	ToolCallID     string
	ToolArguments  string
	ToolResult     string
	ToolDurationMs int64
	ToolErrorCode  string

	// Sub-task board (kind=sub_task_board)
	ChildBoardID string

	// Spirit extension fields
	SpiritSessionID string
	TeamID          string
	DagNodeID       string
	DependsOn       []string

	// Agent info
	AgentKey  string
	AgentName string

	// Display hints
	Collapsed bool
	Label     string
}

// ActivityReader provides read access to Activity records.
// Stability:evolving
type ActivityReader interface {
	ListBySessionTurn(ctx context.Context, sessionID, turnID string) ([]Activity, error)
	ListBySession(ctx context.Context, sessionID string) ([]Activity, error)
	GetActivity(ctx context.Context, id string) (Activity, error)
}

// ActivityWriter provides write access to Activity records.
// Stability:evolving
type ActivityWriter interface {
	CreateActivity(ctx context.Context, a Activity) (Activity, error)
	UpdateActivity(ctx context.Context, a Activity) (Activity, error)
	UpsertActivity(ctx context.Context, a Activity) (Activity, error)
}

// ActivityRepo is the composite interface for Wire binding.
// Stability:evolving
type ActivityRepo interface {
	ActivityReader
	ActivityWriter
}
