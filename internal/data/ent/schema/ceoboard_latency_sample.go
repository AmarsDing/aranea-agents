package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CeoboardLatencySample maps ceoboard_latency_samples rows
// (整合 P2，自 ceo-dashboard SQLite latency_samples 表).
//
// 原表主键 (metric_id, b_uid)（B 侧幂等）；ent 要求单列主键，
// 故用代理 id + 唯一索引表达同一约束（upsert 按该唯一键冲突合并）。
type CeoboardLatencySample struct {
	ent.Schema
}

func (CeoboardLatencySample) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ceoboard_latency_samples"},
	}
}

func (CeoboardLatencySample) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("metric_id").MaxLen(128),
		field.String("b_uid").MaxLen(512).
			Comment("B 侧（后段）事件 uid"),
		field.String("entity_id").Default("").MaxLen(256),
		field.String("a_uid").Default("").MaxLen(512).
			Comment("配对的 A 侧（前段）事件 uid"),
		field.Int64("latency_ms"),
		field.Time("at"),
	}
}

func (CeoboardLatencySample) Indexes() []ent.Index {
	return []ent.Index{
		// 原表 PRIMARY KEY (metric_id, b_uid) 的唯一性约束。
		index.Fields("metric_id", "b_uid").Unique(),
		index.Fields("metric_id", "at"),
	}
}
