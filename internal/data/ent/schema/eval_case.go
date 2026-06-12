package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EvalCase maps table eval_cases.
type EvalCase struct {
	ent.Schema
}

func (EvalCase) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "eval_cases"},
	}
}

func (EvalCase) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("dataset_id"),
	}
}

func (EvalCase) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("dataset_id"),
		field.String("input"),
		field.String("expected_output").Default(""),
		field.String("metadata_json").Default("{}"),
	}
}

func (EvalCase) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("dataset", EvalDataset.Type).
			Ref("cases").
			Field("dataset_id").
			Unique().
			Required(),
		edge.To("results", EvalCaseResult.Type),
	}
}
