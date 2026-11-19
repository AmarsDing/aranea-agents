package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CeoboardWebhookDelivery maps ceoboard_webhook_deliveries rows
// (整合 P2，自 ceo-dashboard SQLite webhook_deliveries 表).
//
// 主键即投递幂等键（强制键 > Idempotency-Key 头 > body.idempotency_key >
// body.event_id > 实体 id > sha256 digest 的优先级链在 biz 层计算）。
type CeoboardWebhookDelivery struct {
	ent.Schema
}

func (CeoboardWebhookDelivery) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ceoboard_webhook_deliveries"},
	}
}

func (CeoboardWebhookDelivery) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().MaxLen(512).StorageKey("key").
			Comment("投递幂等键"),
		field.String("source").MaxLen(64),
		field.String("event").Default("").MaxLen(128),
		field.String("entity_id").Default("").MaxLen(256),
		field.Time("at"),
		field.String("digest").Default("").MaxLen(64).
			Comment("body sha256 前 16 hex"),
		field.Text("fields").Default("{}").
			Comment("提取字段 JSON（TEXT 保真，如 opportunity.*）"),
		field.String("status").Default("ok").MaxLen(32).
			Comment("ok|failed"),
		field.String("error").Default("").MaxLen(1024),
	}
}

func (CeoboardWebhookDelivery) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "at"),
	}
}
