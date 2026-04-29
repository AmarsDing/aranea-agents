package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
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

func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("session_id").MaxLen(256),
		field.String("parent_message_id").Default(""),
		field.Int("turn_index").Default(0),
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
