package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// AgentPromptFile maps legacy table agent_prompt_files.
type AgentPromptFile struct {
	ent.Schema
}

func (AgentPromptFile) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agent_prompt_files"},
	}
}

func (AgentPromptFile) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("agent_id").MaxLen(256),
		field.String("file_name").MaxLen(512),
		field.Text("body").Default(""),
		field.Int("sort_order").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
