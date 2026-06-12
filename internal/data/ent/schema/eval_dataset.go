package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// EvalDataset maps table eval_datasets.
type EvalDataset struct {
	ent.Schema
}

func (EvalDataset) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "eval_datasets"},
	}
}

func (EvalDataset) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("name"),
		field.String("description").Default(""),
		field.Int("case_count").Default(0),
		field.String("workspace").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (EvalDataset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("cases", EvalCase.Type),
		edge.To("runs", EvalRun.Type),
	}
}
