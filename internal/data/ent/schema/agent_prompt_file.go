package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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

func (AgentPromptFile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_id", "sort_order").StorageKey("idx_agent_prompt_files_agent"),
	}
}
