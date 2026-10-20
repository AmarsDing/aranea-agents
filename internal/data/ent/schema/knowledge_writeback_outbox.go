package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// KnowledgeWriteBackOutbox is the durable retry queue for knowledge write-back
// batches (G-10, 2026-10-19): AutoMemory 写回团队库失败落 payload，后台 worker
// 周期重放至 WriteBackSessionFacts（AS-EVT-01 Important 分级）。
// Table is created by DDL migration 20261280; this schema documents the contract for
// Ent codegen. Runtime access uses the raw-SQL repo in internal/data
// (knowledge_writeback_outbox.go).
type KnowledgeWriteBackOutbox struct {
	ent.Schema
}

func (KnowledgeWriteBackOutbox) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "knowledge_writeback_outbox"},
	}
}

func (KnowledgeWriteBackOutbox) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("batch_key").MaxLen(128).NotEmpty().Unique().Immutable(),
		field.String("workspace_id").MaxLen(128).Default(""),
		field.Text("payload"),
		field.String("status").MaxLen(16).Default("pending"),
		field.Int("attempts").Default(0),
		field.Text("last_error").Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("published_at").Optional().Nillable(),
	}
}

func (KnowledgeWriteBackOutbox) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
	}
}
