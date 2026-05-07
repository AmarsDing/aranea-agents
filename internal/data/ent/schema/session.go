package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
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

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("owner_type").Default("agent"),
		field.String("agent_id").Default(""),
		field.String("team_id").Default(""),
		field.String("title"),
		field.Text("summary").Default(""),
		field.Float("context_used_ratio").Default(0),
		field.Int("context_used_tokens").Default(0),
		field.Float("max_context_used_ratio").Default(0),
		field.Int("last_context_window_tokens").Default(0),
		field.String("context_status").Default("normal"),
		field.String("dialog_mode").Default(""),
		field.String("provider").Default(""),
		field.String("model").Default(""),
		field.String("status").Default("active"),
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
		field.String("last_message_at").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("archived_at").Default(""),
		field.String("deleted_at").Default(""),
		// Serialized ADK session.Service state (events + KV state) for Runner; empty when legacy chat only.
		field.Text("adk_snapshot_json").Default(""),
	}
}
