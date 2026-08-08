package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EvalRunPreference maps table eval_run_preferences (P3-3 pairwise judging:
// human preference between two runs of one dataset).
type EvalRunPreference struct {
	ent.Schema
}

func (EvalRunPreference) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "eval_run_preferences"},
	}
}

func (EvalRunPreference) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("dataset_id"),
	}
}

func (EvalRunPreference) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("dataset_id"),
		field.String("run_id_a"),
		field.String("run_id_b"),
		// winner_run_id must equal run_id_a or run_id_b.
		field.String("winner_run_id"),
		field.Text("comment").Default(""),
		field.String("created_by").Default(""),
		field.String("created_at").Default(""),
	}
}
