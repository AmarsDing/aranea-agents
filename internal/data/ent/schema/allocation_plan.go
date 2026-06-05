package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AllocationPlan struct {
	ent.Schema
}

func (AllocationPlan) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "allocation_plans"}}
}

func (AllocationPlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(256).Immutable().Unique(),
		field.String("task_plan_id").MaxLen(256).Default(""),
		field.String("spirit_session_id").MaxLen(256).Default(""),
		field.String("trace_id").MaxLen(128).Default(""),
		field.Text("allocations_json").Default("[]"),
		field.String("status").MaxLen(32).Default("draft"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (AllocationPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("spirit_session_id"),
	}
}
