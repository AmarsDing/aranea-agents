package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// PlatformHook maps table hooks (legacy platform "hooks"; schema type cannot be named Hook — Ent reserved).
type PlatformHook struct {
	ent.Schema
}

func (PlatformHook) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "hooks"},
	}
}

func (PlatformHook) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("hook_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
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
