package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// KnowledgeAccessLog 检索命中日志（自治理图谱 M1 数据源 + G-15 审计足迹）：
// 每次检索命中按文档记一行——collection/doc/query_hash 支撑 base-level 激活分
// 与 Hebbian 共激活；G-15 补充会话/用户/Agent 与 rank/score/latency 审计列。
// Table is created by DDL migration 20261220 + 20261282; this schema documents the
// contract for Ent codegen. Runtime access uses the raw-SQL repo in internal/data
// (knowledge_access_log.go) because inserts are batched via UNNEST.
type KnowledgeAccessLog struct {
	ent.Schema
}

func (KnowledgeAccessLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "knowledge_access_log"},
	}
}

func (KnowledgeAccessLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("collection_id").NotEmpty(),
		field.String("doc_id").NotEmpty(),
		field.Time("accessed_at").Default(time.Now).Immutable(),
		field.String("query_hash").Optional(),
		field.String("session_id").Optional(),
		// G-15 审计足迹：请求上下文透传（空串=未知来源）+ 首个 chunk 的最终排位分。
		field.String("user_id").Optional().Default(""),
		field.String("agent_id").Optional().Default(""),
		field.Int("rank").Default(0),
		field.Float("score").Default(0),
		field.Int("latency_ms").Default(0),
	}
}

func (KnowledgeAccessLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("collection_id", "doc_id", "accessed_at"),
		index.Fields("user_id", "accessed_at"),
	}
}
