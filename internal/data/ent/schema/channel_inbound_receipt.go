package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChannelInboundReceipt maps legacy table channel_inbound_receipt.
type ChannelInboundReceipt struct {
	ent.Schema
}

func (ChannelInboundReceipt) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_inbound_receipt"},
	}
}

func (ChannelInboundReceipt) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_id", "idempotency_key").StorageKey("idx_channel_inbound_receipt_idem").Unique(),
	}
}

func (ChannelInboundReceipt) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("channel_id").Default(""),
		field.String("idempotency_key").Default(""),
		field.String("peer_id").Default(""),
		field.String("text_preview").Default(""),
		field.String("created_at").Default(""),
	}
}
