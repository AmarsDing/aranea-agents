package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionRunCheckpoint maps legacy table session_run_checkpoints.
type SessionRunCheckpoint struct {
	ent.Schema
}

func (SessionRunCheckpoint) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "session_run_checkpoints"},
	}
}

func (SessionRunCheckpoint) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_run_id"),
		index.Fields("session_id"),
	}
}

func (SessionRunCheckpoint) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("session_run_id").Default(""),
		field.String("session_id").Default(""),
		field.String("turn_id").Default(""),
		field.String("agent_id").Default(""),
		field.Text("payload_json").Default(""),
		field.String("created_at").Default(""),
	}
}
