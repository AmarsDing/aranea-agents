package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DecisionRecordOutbox is the async-write outbox for decision records (M80, E5).
// Table is created by DDL migration 20261251; this schema documents the contract for
// Ent codegen. Runtime access uses the raw-SQL repo (internal/data decision repo).
type DecisionRecordOutbox struct {
	ent.Schema
}

func (DecisionRecordOutbox) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "decision_record_outbox"},
	}
}

func (DecisionRecordOutbox) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("decision_key").MaxLen(36).NotEmpty().Unique().Immutable(),
		field.Text("payload"),
		field.String("status").MaxLen(16).Default("pending"),
		field.Int("attempts").Default(0),
		field.Text("last_error").Default(""),
		field.String("created_at").MaxLen(40).Default(""),
		field.String("published_at").MaxLen(40).Optional().Nillable(),
	}
}

func (DecisionRecordOutbox) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at").StorageKey("idx_decision_record_outbox_status_created"),
	}
}
