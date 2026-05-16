package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GraphTask struct {
	ent.Schema
}

func (GraphTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_tasks"},
	}
}

func (GraphTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("node_id").MaxLen(128),
		field.String("execution_id").MaxLen(64),
		field.String("assignee").MaxLen(128).Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Text("context").Default("{}"),
		field.Text("input").Default("{}"),
		field.Text("output").Default(""),
		field.Text("summary").Default(""),
		field.Text("metadata").Default("{}"),
		field.String("required_role").MaxLen(128).Default(""),
		field.String("assignment_mode").MaxLen(32).Default("static"),
		field.String("assignment_strategy").MaxLen(32).Default(""),
		field.Time("created_at"),
		field.Time("claimed_at").Optional().Nillable(),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (GraphTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("execution_id"),
		index.Fields("status"),
		index.Fields("assignee"),
		index.Fields("execution_id", "status"),
	}
}
