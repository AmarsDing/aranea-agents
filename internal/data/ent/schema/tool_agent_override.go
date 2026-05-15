package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ToolAgentOverride struct {
	ent.Schema
}

func (ToolAgentOverride) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tool_agent_overrides"},
	}
}

func (ToolAgentOverride) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("tool_id").Default(""),
		field.String("tool_key"),
		field.String("agent_id"),
		field.Bool("enabled").Default(true),
		field.String("mode").Default("inherit"),
		field.Text("config_override_json").Default("{}"),
		field.Bool("requires_confirmation").Default(false),
		field.String("created_at"),
		field.String("updated_at"),
		field.String("deleted_at").Default(""),
	}
}

func (ToolAgentOverride) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tool_key", "agent_id").Unique(),
		index.Fields("agent_id"),
		index.Fields("tool_key"),
	}
}
