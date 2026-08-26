package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MemoryFactPending is one pending high-risk memory write awaiting human
// approval (79-runtime-governance R3). UPDATE / DELETE / contested verdicts
// from the automatic fact write pipeline land here (status=pending) instead
// of writing directly; the approval center decision executes the original
// bi-temporal write or marks the row rejected.
// Table is created by DDL migration 20261249; this schema documents the
// contract for Ent codegen. Runtime access uses the raw-SQL repo.
type MemoryFactPending struct {
	ent.Schema
}

func (MemoryFactPending) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "memory_fact_pending"},
	}
}

func (MemoryFactPending) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(36).NotEmpty().Unique().Immutable(),
		field.String("agent_id").MaxLen(64).NotEmpty(),
		field.String("fact_key").MaxLen(255).Default(""),
		// verdict: UPDATE | DELETE | CONTESTED.
		field.String("verdict").MaxLen(16).Default(""),
		field.Text("proposed_body").Default(""),
		field.Text("prior_body").Default(""),
		field.Text("adjudicator_reason").Default(""),
		// status: pending | approved | rejected.
		field.String("status").MaxLen(16).Default("pending"),
		field.String("approver").MaxLen(128).Default(""),
		// Unix seconds; decided_at = 0 while pending.
		field.Int64("created_at").Default(0),
		field.Int64("decided_at").Default(0),
	}
}

func (MemoryFactPending) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at").StorageKey("idx_mfp_status"),
		index.Fields("agent_id", "status").StorageKey("idx_mfp_agent_status"),
	}
}
