package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChannelTurnJob maps legacy table channel_turn_job.
type ChannelTurnJob struct {
	ent.Schema
}

func (ChannelTurnJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_turn_job"},
	}
}

func (ChannelTurnJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_id", "idempotency_key").StorageKey("idx_channel_turn_job_idem").Unique(),
		index.Fields("channel_id"),
		index.Fields("session_id"),
	}
}

func (ChannelTurnJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("channel_id").Default(""),
		field.String("session_id").Default(""),
		field.String("peer_id").Default(""),
		field.String("peer_key").Default(""),
		field.String("idempotency_key").Default(""),
		field.String("status").Default("accepted"),
		field.String("preview_message_id").Default(""),
		field.Text("content_preview").Default(""),
		field.String("async_target_type").Default(""),
		field.String("async_target_id").Default(""),
		field.Text("error_message").Default(""),
		field.String("started_at").Default(""),
		field.String("finished_at").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
