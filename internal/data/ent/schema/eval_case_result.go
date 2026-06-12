package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EvalCaseResult maps table eval_case_results.
type EvalCaseResult struct {
	ent.Schema
}

func (EvalCaseResult) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "eval_case_results"},
	}
}

func (EvalCaseResult) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_id"),
		index.Fields("case_id"),
	}
}

func (EvalCaseResult) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("run_id"),
		field.String("case_id"),
		field.Text("actual_output").Default(""),
		field.Int("exact_match").Default(0),
		field.Int("contains_match").Default(0),
		field.Float("llm_judge_score").Default(0),
		field.Float("tool_call_accuracy").Default(0),
		field.Text("error_message").Default(""),
		field.String("created_at").Default(""),
		field.Int("human_pass").Optional().Nillable(),
		field.Float("human_score").Optional().Nillable(),
		field.String("human_comment").Default(""),
		field.String("annotated_at").Default(""),
		field.String("annotated_by").Default(""),
		field.Text("scores_json").Default("{}"),
	}
}

func (EvalCaseResult) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("run", EvalRun.Type).
			Ref("results").
			Field("run_id").
			Unique(),
		edge.From("case", EvalCase.Type).
			Ref("results").
			Field("case_id").
			Unique(),
	}
}
