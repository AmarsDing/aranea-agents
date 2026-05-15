package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ToolInvocationParam struct {
	ent.Schema
}

func (ToolInvocationParam) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tool_invocation_params"},
	}
}

func (ToolInvocationParam) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("invocation_id"),
		field.String("tool_key"),
		field.Text("params_json").Default("{}"),
		field.Bool("redaction_applied").Default(true),
		field.String("created_at"),
	}
}

func (ToolInvocationParam) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invocation_id"),
		index.Fields("tool_key"),
	}
}
