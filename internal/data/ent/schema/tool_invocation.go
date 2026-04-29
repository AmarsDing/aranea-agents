package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// ToolInvocation maps legacy table tool_invocations.
type ToolInvocation struct {
	ent.Schema
}

func (ToolInvocation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tool_invocations"},
	}
}

func (ToolInvocation) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("request_id").Default(""),
		field.String("invocation_id").Default(""),
		field.String("tool_id").Default(""),
		field.String("tool_key"),
		field.String("agent_id").Default(""),
		field.String("agent_key").Default(""),
		field.String("session_id").Default(""),
		field.String("message_id").Default(""),
		field.String("user_id").Default(""),
		field.String("source").Default("adk"),
		field.String("status").Default("success"),
		field.String("started_at"),
		field.String("ended_at").Default(""),
		field.Int("duration_ms").Default(0),
		field.Text("input_preview").Default(""),
		field.String("input_hash").Default(""),
		field.Text("output_preview").Default(""),
		field.String("output_hash").Default(""),
		field.String("error_code").Default(""),
		field.Text("error_message").Default(""),
		field.Bool("redaction_applied").Default(true),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at"),
	}
}
