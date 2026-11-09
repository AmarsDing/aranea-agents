package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CeoboardEvent maps ceoboard_events rows (整合 P2，自 ceo-dashboard SQLite events 表).
//
// 主键即 event_uid = platform + ":" + 去重键（webhook 与 NATS 同表同源）。
// payload 保持 TEXT 而非 jsonb——jsonb 会重排键序，破坏原文保真与脱敏重放语义
// （移植风险点：归档/脱敏/重放依赖原文）。
type CeoboardEvent struct {
	ent.Schema
}

func (CeoboardEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ceoboard_events"},
	}
}

func (CeoboardEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().MaxLen(512).StorageKey("event_uid").
			Comment("event_uid = platform + \":\" + dedupe key"),
		field.String("platform").MaxLen(64),
		field.String("subject").MaxLen(256),
		field.String("subject_key").MaxLen(256).
			Comment("subject 剥离 .vN 后缀的归并键"),
		field.String("event_id").Default("").MaxLen(256),
		field.String("entity_id").Default("").MaxLen(256),
		field.Time("occurred_at").Optional().Nillable().
			Comment("信封发生时间；NULL = 信封未带时间（原系统存 0）"),
		field.Time("received_at"),
		field.Text("payload").Default("").
			Comment("原始报文 TEXT（保真，不用 jsonb）"),
		field.Bool("imported_from_spool").Default(false),
		field.Bool("payload_complete").Default(false).
			Comment("false=被截断；同 uid 补全时单调升级为 true"),
	}
}

func (CeoboardEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subject_key", "received_at"),
		// 评审 2026-11-09 D1：原 (platform, received_at) 两列索引是下方三列
		// 索引的左前缀（PG/SQLite btree 左前缀规则同服务），冗余——写入放大
		// 且占存储，删除收敛；存量库由迁移 20261312 DROP INDEX 收敛。
		// 游标分页主查询 ORDER BY received_at DESC, event_uid DESC
		// （PG btree 反向扫描即可，无需显式 DESC 索引）。
		index.Fields("platform", "received_at", "id"),
		index.Fields("platform", "subject", "received_at"),
		index.Fields("platform", "event_id"),
	}
}
