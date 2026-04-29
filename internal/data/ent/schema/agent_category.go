package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// AgentCategory maps table agent_category_nodes (legacy platform "agent-categories").
type AgentCategory struct {
	ent.Schema
}

func (AgentCategory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agent_category_nodes"},
	}
}

func (AgentCategory) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("category_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.String("parent_id").Default(""),
		field.String("level").Default(""),
		field.String("workspace_id").Default(""),
		field.String("owner_user_id").Default(""),
		field.Bool("is_system").Default(false),
		field.Text("config_json").Default(""),
		field.Text("metadata_json").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
