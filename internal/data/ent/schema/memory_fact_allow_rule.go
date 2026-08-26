package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MemoryFactAllowRule is a persisted approve_always grant for memory
// high-risk writes (79-runtime-governance R3 Phase 3.4, E4): while a rule
// row exists, same (agent_id, verdict) writes bypass the pending gate and
// write directly (still audited). approve_session grants are process-local
// and never land here.
// Table is created by DDL migration 20261256; this schema documents the
// contract for Ent codegen. Runtime access uses the raw-SQL repo.
type MemoryFactAllowRule struct {
	ent.Schema
}

func (MemoryFactAllowRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "memory_fact_allow_rules"},
	}
}

func (MemoryFactAllowRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(36).NotEmpty().Unique().Immutable(),
		field.String("agent_id").MaxLen(64).NotEmpty(),
		// verdict: UPDATE | DELETE | CONTESTED.
		field.String("verdict").MaxLen(16).Default(""),
		// created_by: approver identity that granted the rule.
		field.String("created_by").MaxLen(128).Default(""),
		// Unix seconds.
		field.Int64("created_at").Default(0),
	}
}

func (MemoryFactAllowRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_id", "verdict").Unique().StorageKey("idx_mfar_agent_verdict"),
	}
}
