package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GraphTaskLink struct {
	ent.Schema
}

func (GraphTaskLink) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_task_links"},
	}
}

func (GraphTaskLink) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("parent_task_id").MaxLen(64),
		field.String("child_task_id").MaxLen(64),
		field.String("execution_id").MaxLen(64),
		field.Time("created_at"),
	}
}

func (GraphTaskLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("parent_task_id"),
		index.Fields("child_task_id"),
		index.Fields("execution_id"),
		index.Fields("parent_task_id", "child_task_id").Unique(),
	}
}
