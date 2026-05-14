package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type EvolutionSuggestion struct {
	ent.Schema
}

func (EvolutionSuggestion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "evolution_suggestions"},
	}
}

func (EvolutionSuggestion) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("agent_id").MaxLen(256),
		field.String("type").MaxLen(64),
		field.String("title").MaxLen(512),
		field.Text("content").Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Text("diff_preview").Default(""),
		field.String("created_at").Default(""),
		field.String("applied_at").Default(""),
	}
}

func (EvolutionSuggestion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_id", "status"),
		index.Fields("agent_id", "created_at"),
	}
}
