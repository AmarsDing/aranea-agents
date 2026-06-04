package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Orchestration struct {
	ent.Schema
}

func (Orchestration) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "orchestrations"}}
}

func (Orchestration) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(256).Immutable().Unique(),
		field.String("task_plan_id").MaxLen(256).Default(""),
		field.String("allocation_id").MaxLen(256).Default(""),
		field.String("spirit_session_id").MaxLen(256).Default(""),
		field.String("trace_id").MaxLen(128).Default(""),
		field.String("strategy").MaxLen(64).Default("direct"),
		field.String("graph_execution_id").MaxLen(256).Default(""),
		field.Text("team_ids_json").Default("[]"),
		field.String("status").MaxLen(32).Default("pending"),
		field.String("checkpoint_id").MaxLen(256).Default(""),
		field.Text("synthesis_result_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (Orchestration) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("spirit_session_id"),
	}
}
