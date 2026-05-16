package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GraphTaskLog struct {
	ent.Schema
}

func (GraphTaskLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_task_logs"},
	}
}

func (GraphTaskLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("stream").MaxLen(16).Default("stdout"),
		field.Text("content"),
		field.String("level").MaxLen(16).Default("info"),
		field.Time("timestamp"),
	}
}

func (GraphTaskLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id"),
		index.Fields("task_id", "stream"),
		index.Fields("task_id", "level"),
	}
}
