package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// MediaProvider maps table media_providers: media generation provider configs.
type MediaProvider struct {
	ent.Schema
}

func (MediaProvider) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "media_providers"},
	}
}

func (MediaProvider) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").NotEmpty().Unique(),
		field.String("name").NotEmpty().Unique(),
		field.String("provider_type").NotEmpty(), // "comfyui_local" / "qwen" / "kling"
		field.String("base_url").Default(""),
		field.String("api_key").Default("").Sensitive(), // DB-N8: sensitive field
		field.String("config_json").Default("{}"),
		field.String("capabilities").Default("[]"), // JSON array: ["image","video","image_to_video"]
		field.String("status").Default("active"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
