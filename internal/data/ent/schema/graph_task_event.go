package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GraphTaskEvent struct {
	ent.Schema
}

func (GraphTaskEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_task_events"},
	}
}

func (GraphTaskEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("event_type").MaxLen(64),
		field.String("source_node").MaxLen(128).Default(""),
		field.Text("description").Default(""),
		field.Time("timestamp"),
	}
}

func (GraphTaskEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id"),
		index.Fields("event_type"),
	}
}
