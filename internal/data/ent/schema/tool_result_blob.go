package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ToolResultBlob struct {
	ent.Schema
}

func (ToolResultBlob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tool_result_blobs"},
	}
}

func (ToolResultBlob) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("session_id").MaxLen(256),
		field.Int("turn_number").Default(0),
		field.String("tool_name").Default(""),
		field.Text("tool_args_summary").Default(""),
		field.Text("full_content").Default(""),
		field.Int("content_size_chars").Default(0),
		field.String("created_at"),
	}
}

func (ToolResultBlob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "turn_number").StorageKey("idx_tool_result_blobs_session_turn"),
	}
}
