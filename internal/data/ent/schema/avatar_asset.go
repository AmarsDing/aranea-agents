package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// AvatarAsset maps the legacy table avatar_assets (pkg/backend 0001_init.sql).
type AvatarAsset struct {
	ent.Schema
}

func (AvatarAsset) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "avatar_assets"},
	}
}

func (AvatarAsset) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("asset_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Bytes("image_data"),
		field.Bytes("thumbnail_data").Optional(),
		field.String("mime_type").Default("image/png"),
		field.String("workspace_id").Default(""),
		field.String("owner_user_id").Default(""),
		field.String("source").Default("system"),
		field.Bool("is_system").Default(false),
		field.Int("file_size_bytes").Default(0),
		field.Int("width_px").Default(0),
		field.Int("height_px").Default(0),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.Text("config_json").Default(""),
		field.Text("metadata_json").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
