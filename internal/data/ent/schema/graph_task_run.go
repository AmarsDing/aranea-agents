package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GraphTaskRun struct {
	ent.Schema
}

func (GraphTaskRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_task_runs"},
	}
}

func (GraphTaskRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.Time("started_at"),
		field.Time("finished_at").Optional().Nillable(),
		field.Int("exit_code").Default(0),
		field.String("log_ref").MaxLen(256).Default(""),
	}
}

func (GraphTaskRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id"),
	}
}
