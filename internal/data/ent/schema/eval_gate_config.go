package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// EvalGateConfig maps table eval_gate_config (P2-1 publish gate: singleton
// row, id always "singleton"). The regression gate runs the configured
// dataset against the configured agent on skill publish / pack install and
// blocks the operation when the score breaches the configured floor/drop.
type EvalGateConfig struct {
	ent.Schema
}

func (EvalGateConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "eval_gate_config"},
	}
}

func (EvalGateConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64),
		// enabled is stored as INTEGER 0/1 to match the raw-SQL eval schema
		// convention (cf. eval_case_results.human_pass).
		field.Int("enabled").Default(0),
		field.String("agent_id").Default(""),
		field.String("dataset_id").Default(""),
		// metric: exact_match | contains_match | llm_as_judge | tool_call_accuracy
		// (or any extended scores_json key).
		field.String("metric").Default("exact_match"),
		// min_score: absolute floor; 0 disables the absolute check.
		field.Float("min_score").Default(0),
		// max_drop: allowed drop vs the latest completed baseline run;
		// 0 disables the relative check.
		field.Float("max_drop").Default(0),
		field.String("updated_at").Default(""),
	}
}
