package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ToolResultReplacement struct {
	ent.Schema
}

func (ToolResultReplacement) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tool_result_replacements"},
	}
}

func (ToolResultReplacement) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("session_id").MaxLen(256),
		field.String("message_id").MaxLen(256),
		field.String("result_blob_id").MaxLen(256),
		field.Text("preview_text").Default(""),
		field.String("replaced_at"),
	}
}

func (ToolResultReplacement) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "message_id").StorageKey("idx_tool_result_replacements_message").Unique(),
	}
}
