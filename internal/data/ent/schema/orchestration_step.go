package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OrchestrationStep maps orchestration_steps activity timeline rows.
type OrchestrationStep struct {
	ent.Schema
}

func (OrchestrationStep) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "orchestration_steps"},
	}
}

func (OrchestrationStep) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("team_run_id").MaxLen(256).Default(""),
		field.String("graph_execution_id").Default(""),
		field.String("node_id").Default(""),
		field.Text("activity_snapshot_json").Default("{}"),
		field.String("status").Default(""),
		field.String("started_at").Default(""),
		field.String("finished_at").Default(""),
		field.String("created_at").Default(""),
	}
}

func (OrchestrationStep) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_run_id", "created_at"),
		index.Fields("node_id"),
	}
}
