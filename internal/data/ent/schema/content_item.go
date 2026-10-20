package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ContentItem is MVP media/commerce domain table `content_items`.
type ContentItem struct {
	ent.Schema
}

func (ContentItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "content_items"},
	}
}

func (ContentItem) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),

		field.String("workspace_id").Default("").MaxLen(256),
		field.String("org_key").Default("").MaxLen(256),
		field.String("source_type").Default("manual").MaxLen(64),
		field.String("source_url").Default("").MaxLen(2048),
		field.String("title").Default("").MaxLen(1024),
		field.Text("body").Default(""),
		field.String("lang").Default("").MaxLen(32),
		field.Text("media_artifact_ids_json").Default("[]"),
		field.String("dedupe_hash").Default("").MaxLen(128),
		field.Float("hot_score").Default(0),
		field.String("status").Default("candidate").MaxLen(64),
		field.Text("raw_payload").Default(""),

		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (ContentItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
