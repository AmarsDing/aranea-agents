package biz

import (
	"context"
	"time"
)

// ActivityKind classifies the semantic type of an Activity.
// Aligned with TaskBoardNodeKind for zero-inference frontend consumption.
type ActivityKind string

const (
	ActivityKindTask         ActivityKind = "task"           // Task description (user/agent perspective)
	ActivityKindThinking     ActivityKind = "thinking"       // Reasoning content
	ActivityKindAction       ActivityKind = "action"         // Tool invocation
	ActivityKindReply        ActivityKind = "reply"          // Agent reply (including final answer)
	ActivityKindSubTaskBoard ActivityKind = "sub_task_board" // Sub-task board (recursive nesting)
	ActivityKindError        ActivityKind = "error"          // Error information
	ActivityKindDelegate     ActivityKind = "delegate"       // Spirit delegates to team
	ActivityKindNotice       ActivityKind = "notice"         // System notification
	ActivityKindConfirm      ActivityKind = "confirm"        // User confirmation required
	ActivityKindPlan         ActivityKind = "plan"           // Multi-step plan
)

// ActivityStatus represents the lifecycle status of an Activity.
type ActivityStatus string

const (
	ActivityStatusPending        ActivityStatus = "pending"
	ActivityStatusRunning        ActivityStatus = "running"
	ActivityStatusToolRunning    ActivityStatus = "tool_running"
	ActivityStatusToolBlocked    ActivityStatus = "tool_blocked"
	ActivityStatusCompleted      ActivityStatus = "completed"
	ActivityStatusFailed         ActivityStatus = "failed"
	ActivityStatusPartialFailure ActivityStatus = "partial_failure"
	ActivityStatusCancelled      ActivityStatus = "cancelled"
	ActivityStatusInterrupted    ActivityStatus = "interrupted"
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
	Seq              int64 // Global emission sequence for stable frontend ordering

	// Token usage (kind=task, root Activity only)
	PromptTokens     int64
	CompletionTokens int64

	// Content fields (by kind)
	Content   string // task/reply/error text
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

	// Meta stores kind-specific metadata (noticeType, toolName, steps, etc.)
	Meta map[string]any
}

// ActivityPlanStep represents a step in a plan Activity.
type ActivityPlanStep struct {
	ID        string         `json:"id"`
	Label     string         `json:"label"`
	Status    ActivityStatus `json:"status"`
	AgentName string         `json:"agentName,omitempty"`
	DependsOn []string       `json:"dependsOn,omitempty"`
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

// ActivityConfirmParams holds parameters for creating a confirm Activity.
type ActivityConfirmParams struct {
	ToolName      string
	ToolArguments string
	Content       string
}

// ActivityEmitter emits Activity events for runtime notifications.
// Implemented by agent.ActivityProjector, accessed via context by plugins/hooks
// that cannot directly import the agent package (e.g. cost_guard, model_router,
// tool_confirmation). The turn/session IDs are derived from the projector's
// internal ProjectMeta (set via OnTurnStart), so callers don't need to pass them.
// Stability:evolving
type ActivityEmitter interface {
	// EmitNotice emits a notice Activity (kind=notice) and immediately completes it.
	EmitNotice(ctx context.Context, content, noticeType string) error
	// EmitConfirmRequest emits a confirm Activity (kind=confirm, status=tool_blocked)
	// and returns the activity ID for later result correlation.
	EmitConfirmRequest(ctx context.Context, params ActivityConfirmParams) (activityID string, err error)
	// EmitConfirmResult updates a confirm Activity with the user's response.
	EmitConfirmResult(ctx context.Context, activityID string, approved bool) error
}

// activityEmitterKey is the context key for ActivityEmitter.
type activityEmitterKey struct{}

// WithActivityEmitter injects an ActivityEmitter into the context.
func WithActivityEmitter(ctx context.Context, e ActivityEmitter) context.Context {
	return context.WithValue(ctx, activityEmitterKey{}, e)
}

// ActivityEmitterFromContext extracts an ActivityEmitter from the context.
// Returns nil if no emitter is present.
func ActivityEmitterFromContext(ctx context.Context) ActivityEmitter {
	e, _ := ctx.Value(activityEmitterKey{}).(ActivityEmitter)
	return e
}
