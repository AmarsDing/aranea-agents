package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EventDeliveryOutbox persists critical v2 delivery events for reconnect replay (B-06).
// Table is created by DDL migration 20261010; this schema documents the contract for
// future Ent codegen. Runtime access uses the raw-SQL repo in internal/data.
type EventDeliveryOutbox struct {
	ent.Schema
}

func (EventDeliveryOutbox) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "event_delivery_outbox"},
	}
}

func (EventDeliveryOutbox) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("session_id").MaxLen(128).NotEmpty(),
		field.Int64("seq"),
		field.String("event_id").MaxLen(256).NotEmpty(),
		field.String("kind").MaxLen(64).Default(""),
		field.String("entity_id").MaxLen(128).Default(""),
		field.Bytes("payload"),
		field.Time("published_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (EventDeliveryOutbox) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "seq").Unique(),
		index.Fields("session_id", "event_id").Unique(),
		index.Fields("session_id"),
	}
}
