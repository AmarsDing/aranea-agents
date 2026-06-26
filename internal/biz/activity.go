package biz

import (
	"context"
	"time"
)

// ActivityKind classifies the semantic type of an Activity.
// Aligned with TaskBoardNodeKind for zero-inference frontend consumption.
type ActivityKind string

const (
	ActivityKindTask     ActivityKind = "task"     // Task description (user/agent perspective)
	ActivityKindThinking ActivityKind = "thinking" // Reasoning content
	ActivityKindAction   ActivityKind = "action"   // Tool invocation
	ActivityKindReply    ActivityKind = "reply"    // Agent reply (including final answer)
	ActivityKindNotice   ActivityKind = "notice"   // System notification
	ActivityKindConfirm  ActivityKind = "confirm"  // User confirmation required
	ActivityKindPlan     ActivityKind = "plan"     // Multi-step plan

	// === Session/Team/Graph lifecycle (Phase 1a additive) ===
	// These kinds allow ActivityProjector to emit session/team/graph stage
	// events as Activities, enabling unified frontend rendering.
	ActivityKindSession    ActivityKind = "session"     // Session lifecycle (created/status/completed)
	ActivityKindTeamStage  ActivityKind = "team_stage"  // Team stage (assembled/executing/completed/failed)
	ActivityKindGraphStage ActivityKind = "graph_stage" // Graph stage (planned/executing/completed/failed)
)

// ToolCategory classifies a tool by its functional type for UI rendering.
// The frontend uses tool_category to pick the appropriate detail component
// (shell terminal, browser card, file diff, etc.) without parsing tool_name.
type ToolCategory string

const (
	ToolCategoryShell      ToolCategory = "shell"       // Shell command execution
	ToolCategoryBrowser    ToolCategory = "browser"     // Browser automation
	ToolCategoryFileRead   ToolCategory = "file_read"   // File read
	ToolCategoryFileWrite  ToolCategory = "file_write"  // File write/edit
	ToolCategoryFileSearch ToolCategory = "file_search" // File search (find/grep/glob)
	ToolCategoryWebSearch  ToolCategory = "web_search"  // Web search
	ToolCategoryMCP        ToolCategory = "mcp"         // MCP tool
	ToolCategoryCode       ToolCategory = "code"        // Code execution
	ToolCategoryTodo       ToolCategory = "todo"        // Todo management
	ToolCategoryOther      ToolCategory = "other"       // Other / unknown
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
//
// JSON tags use snake_case to match the ActivityEvent contract consumed by
// the Activity-First frontend (web/src/realtime/activityEvent.ts). The WS
// path serializes biz.Activity directly inside biz.ActivityEvent, so these
// tags are the source of truth for field names on the wire.
type Activity struct {
	ID               string         `json:"id"`
	Kind             ActivityKind   `json:"kind"`
	Status           ActivityStatus `json:"status"`
	SessionID        string         `json:"session_id"`
	TurnID           string         `json:"turn_id"`
	ParentActivityID string         `json:"parent_activity_id,omitempty"`
	Timestamp        time.Time      `json:"timestamp"`
	DurationMs       int64          `json:"duration_ms"`
	Seq              int64          `json:"seq"` // Global emission sequence for stable frontend ordering

	// Token usage (kind=task, root Activity only)
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`

	// Content fields (by kind)
	Content   string `json:"content,omitempty"`   // task/reply/error text
	Reasoning string `json:"reasoning,omitempty"` // thinking reasoning content

	// Tool fields (kind=action)
	ToolName       string       `json:"tool_name,omitempty"`
	ToolCategory   ToolCategory `json:"tool_category,omitempty"` // Tool functional category for UI rendering
	ToolCallID     string       `json:"tool_call_id,omitempty"`
	ToolArguments  string       `json:"tool_arguments,omitempty"`
	ToolResult     string       `json:"tool_result,omitempty"`
	ToolDurationMs int64        `json:"tool_duration_ms"`
	ToolErrorCode  string       `json:"tool_error_code,omitempty"`

	// Stage (kind=session/team_stage/graph_stage)
	// Represents the current phase: assembled/planning/executing/completed/failed etc.
	Stage string `json:"stage,omitempty"`

	// Sub-task board (kind=sub_task_board)
	ChildBoardID string `json:"child_board_id,omitempty"`

	// Spirit extension fields
	SpiritSessionID string   `json:"spirit_session_id,omitempty"`
	TeamID          string   `json:"team_id,omitempty"`
	DagNodeID       string   `json:"dag_node_id,omitempty"`
	DependsOn       []string `json:"depends_on,omitempty"`

	// Agent info
	AgentKey  string `json:"agent_key,omitempty"`
	AgentName string `json:"agent_name,omitempty"`

	// Display hints
	Collapsed bool   `json:"collapsed"`
	Label     string `json:"label,omitempty"`

	// Meta stores kind-specific metadata (noticeType, toolName, steps, etc.)
	Meta map[string]any `json:"meta,omitempty"`
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

	// ListBySpiritSession returns all activities under a spirit session tree
	// (across team/agent sub-sessions). Uses spirit_session_id index.
	ListBySpiritSession(ctx context.Context, spiritSessionID string) ([]Activity, error)

	// ListByTeam returns all activities for a given team.
	ListByTeam(ctx context.Context, teamID string) ([]Activity, error)

	// ListByParentSession returns activities whose session_id belongs to direct
	// child sessions of parentSessionID. Used for member session activity loading.
	ListByParentSession(ctx context.Context, parentSessionID string) ([]Activity, error)
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
