package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChannelRuntimeLease maps legacy table channel_runtime_lease.
type ChannelRuntimeLease struct {
	ent.Schema
}

func (ChannelRuntimeLease) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_runtime_lease"},
	}
}

func (ChannelRuntimeLease) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("expires_at").StorageKey("idx_channel_runtime_lease_expires"),
	}
}

func (ChannelRuntimeLease) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Immutable().Unique().MaxLen(256),
		field.String("channel_id").Default(""),
		field.String("platform").Default(""),
		field.String("owner_id").Default(""),
		field.String("expires_at").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
