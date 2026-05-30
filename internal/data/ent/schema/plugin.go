package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformPlugin maps legacy table plugins (Ent type cannot be named Plugin — clashes with Go plugin).
type PlatformPlugin struct {
	ent.Schema
}

func (PlatformPlugin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "plugins"},
	}
}

func (PlatformPlugin) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("plugin_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("category").Default(""),
		field.String("risk_level").Default("low"),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(false),
		field.String("scope").Default("global").MaxLen(64),
		field.Text("callback_points_json").Default("[]"),
		field.Int("sort_order").Default(0),
		field.Text("config_schema_json").Default("{}"),
		field.Text("config_json").Default("{}"),
		// default_config_json — field name avoids Ent global DefaultConfigJSON (config_json default) collision.
		field.Text("fallback_config_json").StorageKey("default_config_json").Default("{}"),
		field.Int("invoke_count").Default(0),
		field.Int("block_count").Default(0),
		field.Int("error_count").Default(0),
		field.String("last_invoked_at").Default(""),
		field.String("last_status").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (PlatformPlugin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "sort_order").StorageKey("idx_plugins_enabled_order"),
	}
}
