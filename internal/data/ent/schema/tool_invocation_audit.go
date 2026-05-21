package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ToolInvocationAudit maps legacy table tool_invocation_audit.
type ToolInvocationAudit struct {
	ent.Schema
}

func (ToolInvocationAudit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tool_invocation_audit"},
	}
}

func (ToolInvocationAudit) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("invocation_id").Default(""),
		field.String("tool_key"),
		field.String("agent_id").Default(""),
		field.String("user_id").Default(""),
		field.String("session_id").Default(""),
		field.String("action").Default("tool.call"),
		field.Text("result_summary").Default(""),
		field.String("status").Default("success"),
		field.String("source").Default("adk"),
		field.String("created_at"),
	}
}

func (ToolInvocationAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tool_key", "created_at"),
		index.Fields("agent_id", "created_at"),
		index.Fields("user_id", "created_at"),
	}
}
