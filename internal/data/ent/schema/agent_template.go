package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AgentTemplate struct {
	ent.Schema
}

func (AgentTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agent_templates"},
	}
}

func (AgentTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("template_key"),
	}
}

func (AgentTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("template_key").Unique().MaxLen(256),
		field.String("label").MaxLen(256),
		field.String("icon").Default("smart_toy"),
		field.String("display_name").Default(""),
		field.String("provider").Default("openrouter"),
		field.String("model").Default("gpt-4.1-mini"),
		field.Text("description").Default(""),
		field.Int("sort_order").Default(0),
		field.Bool("is_system").Default(true),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
