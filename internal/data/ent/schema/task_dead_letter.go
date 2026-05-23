package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TaskDeadLetter maps task_dead_letters rows (M53 FP-04).
type TaskDeadLetter struct {
	ent.Schema
}

func (TaskDeadLetter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "task_dead_letters"},
	}
}

func (TaskDeadLetter) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("source_type").Default(""),
		field.String("source_id").Default(""),
		field.String("team_id").Default(""),
		field.String("team_run_id").Default(""),
		field.String("session_id").Default(""),
		field.String("graph_execution_id").Default(""),
		field.Text("error_message").Default(""),
		field.Text("payload_json").Default("{}"),
		field.String("status").Default("pending"),
		field.String("created_at").Default(""),
		field.String("resolved_at").Default(""),
	}
}

func (TaskDeadLetter) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("team_run_id"),
	}
}
