package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EventStore persists Envelope snapshots for cross-restart replay.
type EventStore struct {
	ent.Schema
}

func (EventStore) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "event_store"},
	}
}

func (EventStore) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("session_id").MaxLen(128).Default(""),
		field.String("type").MaxLen(64),
		field.String("author").MaxLen(128).Default(""),
		field.String("channel").MaxLen(32).Default(""),
		field.Text("envelope_json"),
		field.Time("created_at"),
	}
}

func (EventStore) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "created_at"),
		index.Fields("type"),
		index.Fields("created_at"),
	}
}
