package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformChannelDelivery maps table `channel_delivery`.
type PlatformChannelDelivery struct {
	ent.Schema
}

func (PlatformChannelDelivery) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_delivery"},
	}
}

func (PlatformChannelDelivery) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("channel_id").MaxLen(256).NotEmpty(),
		field.String("agent_id").Default(""),
		field.String("status").Default("pending"),
		field.Text("payload_json").Default("{}"),
		field.Text("error_message").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (PlatformChannelDelivery) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_id", "created_at").StorageKey("idx_channel_delivery_channel"),
	}
}
