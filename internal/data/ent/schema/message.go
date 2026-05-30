package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Message maps legacy table messages.
type Message struct {
	ent.Schema
}

func (Message) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "messages"},
	}
}

func (Message) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("session_id", "turn_id"),
		index.Fields("session_id", "turn_number").StorageKey("idx_messages_session_turn"),
		index.Fields("session_id", "status"),
	}
}

func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("session_id").MaxLen(256),
		field.String("parent_message_id").Default(""),
		field.String("turn_id").Default("").MaxLen(256),
		field.Int("turn_number").Default(0),
		field.Int("seq_in_turn").Default(0),
		field.String("role"),
		field.Text("content_markdown").Default(""),
		field.String("model_name").Default(""),
		field.Int("token_in").Default(0),
		field.Int("token_out").Default(0),
		field.Int("latency_ms").Default(0),
		field.String("status").Default("ok"),
		field.Int("attachments_count").Default(0),
		field.Text("options_json").Default(""),
		field.Text("error_message").Default(""),
		field.String("created_at"),
	}
}
