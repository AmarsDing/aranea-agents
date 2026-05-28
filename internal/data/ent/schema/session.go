package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Session maps legacy table sessions（包含 sqlite.ensureLegacyColumns 增补列）.
type Session struct {
	ent.Schema
}

func (Session) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sessions"},
	}
}

func (Session) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("deleted_at", "agent_id"),
		index.Fields("deleted_at", "user_id"),
		index.Fields("deleted_at", "status"),
	}
}

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),

		field.String("workspace_id").Default(""),
		field.String("user_id").Default(""),
		field.String("owner_type").Default("agent"),
		field.String("agent_id").Default(""),
		field.String("team_id").Default(""),

		field.String("title"),
		field.Text("summary").Default(""),
		field.Text("tags_json").Default("[]"),

		field.String("dialog_mode").Default(""),
		field.String("default_provider").Default(""),
		field.String("default_model").Default(""),
		field.Int("default_context_window_tokens").Default(0),

		field.String("last_provider").Default(""),
		field.String("last_model").Default(""),
		field.Int("last_context_window_tokens").Default(0),

		field.String("status").Default("active"),
		field.String("visibility").Default("private"),

		field.Int("message_count").Default(0),
		field.Int("run_count").Default(0),
		field.Int("model_call_count").Default(0),
		field.Int("tool_call_count").Default(0),
		field.Int("skill_call_count").Default(0),
		field.Int("mcp_call_count").Default(0),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.Int("total_tokens").Default(0),
		field.Int64("total_cost_micro_usd").Default(0),
		field.Float("avg_latency_ms").Default(0.0),
		field.Int("error_count").Default(0),

		field.Int("context_used_tokens").Default(0),
		field.Float("context_used_ratio").Default(0.0),
		field.Float("max_context_used_ratio").Default(0.0),
		field.String("context_status").Default("normal"),

		field.String("first_message_at").Default(""),
		field.String("last_message_at").Default(""),
		field.String("last_run_at").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("archived_at").Default(""),
		field.String("deleted_at").Default(""),
		field.String("pinned_at").Default(""),

		field.Text("runner_snapshot_json").Default(""),
		field.Text("state_json").Default("{}"),
		field.Text("metadata_json").Default("{}"),
		field.Int64("session_revision").Default(0),
	}
}
