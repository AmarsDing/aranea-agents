package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionParticipant maps legacy table session_participants.
type SessionParticipant struct {
	ent.Schema
}

func (SessionParticipant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "session_participants"},
	}
}

func (SessionParticipant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("participant_id").StorageKey("idx_session_participants_participant"),
	}
}

func (SessionParticipant) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("session_id").MaxLen(256),
		field.String("participant_type").Default(""),
		field.String("participant_id").Default(""),
		field.String("display_name").Default(""),
		field.String("role_in_session").Default(""),
		field.String("status").Default("active"),
		field.String("first_active_at").Default(""),
		field.String("last_active_at").Default(""),
		field.Int("message_count").Default(0),
		field.Int("run_step_count").Default(0),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.Float("context_used_ratio").Default(0.0),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
