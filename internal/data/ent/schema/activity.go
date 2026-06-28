package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Activity persists projected Activity lifecycle records for the Activity-First architecture.
// Each row represents a semantic unit (thinking/reply/action/delegate/etc.) that the backend
// projects from runtime events and pushes to the frontend via WS, eliminating frontend inference.
type Activity struct {
	ent.Schema
}

func (Activity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "activities"},
	}
}

func (Activity) Fields() []ent.Field {
	return []ent.Field{
		// === Primary key ===
		field.String("id").MaxLen(64).Unique().Immutable(),

		// === Classification ===
		field.String("kind").MaxLen(32).Comment("ActivityKind: task/thinking/action/reply/sub_task_board/error/delegate/notice/confirm/plan"),
		field.String("status").MaxLen(32).Default("pending").Comment("ActivityStatus: pending/running/tool_running/tool_blocked/completed/failed/partial_failure/cancelled/interrupted"),

		// === Ownership ===
		field.String("session_id").MaxLen(128).Default(""),
		field.String("turn_id").MaxLen(128).Default(""),
		field.String("parent_activity_id").MaxLen(64).Default("").Comment("FK to parent Activity for tree nesting"),

		// === Timing ===
		field.String("timestamp").Default("").Comment("ISO8601 start timestamp"),
		field.Int64("duration_ms").Default(0).Comment("Duration in ms, filled on completion"),
		field.Int64("seq").Default(0).Comment("Global emission sequence for stable frontend ordering"),

		// === Token usage (kind=task, root Activity only) ===
		field.Int64("prompt_tokens").Default(0).Comment("LLM prompt tokens for this turn (root task only)"),
		field.Int64("completion_tokens").Default(0).Comment("LLM completion tokens for this turn (root task only)"),

		// === Content fields (by kind) ===
		field.Text("content").Default("").Comment("task/reply/error text content"),
		field.Text("reasoning").Default("").Comment("thinking reasoning content"),

		// === Tool fields (kind=action) ===
		field.String("tool_name").MaxLen(128).Default(""),
		field.String("tool_category").MaxLen(32).Default("").Comment("ToolCategory: shell/browser/file_read/file_write/file_search/web_search/mcp/code/todo/other"),
		field.String("tool_call_id").MaxLen(512).Default("").Comment("LLM tool call ID (some providers exceed 128 chars)"),
		field.Text("tool_arguments").Default("").Sensitive().Comment("JSON tool arguments, sensitive"),
		field.Text("tool_result").Default("").Sensitive().Comment("JSON tool result, sensitive"),
		field.Int64("tool_duration_ms").Default(0),
		field.String("tool_error_code").MaxLen(64).Default("").Comment("e.g. tool_timeout, tool_error"),

		// === Stage (kind=session/team_stage/graph_stage) ===
		field.String("stage").MaxLen(64).Default("").Comment("Current phase: assembled/planning/executing/completed/failed etc."),

		// === Sub-task board (kind=sub_task_board) ===
		field.String("child_board_id").MaxLen(64).Default("").Comment("Root Activity ID of child board"),

		// === Spirit extension fields ===
		field.String("spirit_session_id").MaxLen(128).Default("").Comment("Spirit Session ID"),
		field.String("team_id").MaxLen(128).Default("").Comment("Associated Team ID"),
		field.String("dag_node_id").MaxLen(128).Default("").Comment("DAG node ID"),
		field.JSON("depends_on", []string{}).Optional().Comment("DAG dependency node IDs"),

		// === Agent info ===
		field.String("agent_key").MaxLen(128).Default(""),
		field.String("agent_name").MaxLen(128).Default(""),

		// === Display hints ===
		field.Bool("collapsed").Default(false).Comment("Backend suggestion, frontend may override"),
		field.String("label").MaxLen(128).Default("").Comment("Custom label e.g. '规划'/'推理'/'重规划'"),

		// === Kind-specific metadata ===
		field.JSON("meta", map[string]any{}).Optional().Comment("Kind-specific metadata (noticeType, toolName, steps, etc.)"),
	}
}

func (Activity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "turn_id").StorageKey("idx_activities_session_turn"),
		index.Fields("parent_activity_id").StorageKey("idx_activities_parent"),
		index.Fields("spirit_session_id").StorageKey("idx_activities_spirit_session"),
		index.Fields("team_id").StorageKey("idx_activities_team"),
	}
}
