package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EvalRun maps table eval_runs.
type EvalRun struct {
	ent.Schema
}

func (EvalRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "eval_runs"},
	}
}

func (EvalRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("dataset_id"),
		index.Fields("agent_id"),
		// P2-B: workspace visibility filter.
		index.Fields("workspace_id").StorageKey("idx_eval_runs_workspace"),
	}
}

func (EvalRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("dataset_id"),
		field.String("agent_id"),
		field.String("status").Default("pending"),
		field.Int("total_cases").Default(0),
		field.Int("completed_cases").Default(0),
		field.Float("exact_match_score").Default(0),
		field.Float("contains_match_score").Default(0),
		field.Float("llm_judge_score").Default(0),
		field.Float("tool_call_accuracy").Default(0),
		field.Float("pass_at_k").Default(0),
		field.Float("pass_hat_k").Default(0),
		field.String("trigger_source").Default("manual"),
		field.Int("num_runs").Default(1),
		field.Text("scores_json").Default("{}"),
		field.Text("error_message").Default(""),
		field.String("started_at").Default(""),
		field.String("finished_at").Default(""),
		// P3-5: dataset content hash snapshot at run start — trend/compare uses
		// it to warn when the dataset changed between runs.
		field.String("dataset_hash").Default("").MaxLen(64),
		// P2-B: tenant isolation. empty = legacy (treated as default workspace);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").MaxLen(128),
		field.String("created_at").Default(""),
	}
}

func (EvalRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("dataset", EvalDataset.Type).
			Ref("runs").
			Field("dataset_id").
			Unique().
			Required(),
		edge.To("results", EvalCaseResult.Type),
	}
}
