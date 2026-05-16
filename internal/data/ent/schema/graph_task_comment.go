package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GraphTaskComment struct {
	ent.Schema
}

func (GraphTaskComment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_task_comments"},
	}
}

func (GraphTaskComment) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("author").MaxLen(128),
		field.Text("content"),
		field.String("type").MaxLen(32).Default("suggestion"),
		field.Time("created_at"),
	}
}

func (GraphTaskComment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id"),
	}
}
