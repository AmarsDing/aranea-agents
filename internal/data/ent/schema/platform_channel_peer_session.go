package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformChannelPeerSession maps chat session to an external channel peer (e.g. Feishu open_id).
// peer_key empty string means dm_scope=main (one session per channel).
type PlatformChannelPeerSession struct {
	ent.Schema
}

func (PlatformChannelPeerSession) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_peer_session"},
	}
}

func (PlatformChannelPeerSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("channel_id").MaxLen(256),
		field.String("peer_key").Default("").MaxLen(1024),
		field.String("session_id").MaxLen(256),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (PlatformChannelPeerSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_id", "peer_key").Unique(),
	}
}
