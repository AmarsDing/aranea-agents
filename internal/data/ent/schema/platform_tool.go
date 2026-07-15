package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformTool maps legacy table tools（capability 目录；Ent 类型名避免单字 Tool）。
type PlatformTool struct {
	ent.Schema
}

func (PlatformTool) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tools"},
	}
}

func (PlatformTool) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("tool_key").Unique().MaxLen(512),
		field.String("display_name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("category").Default("system"),
		field.String("source").Default("builtin"),
		field.String("risk_level").Default("low"),
		field.Bool("enabled").Default(true),
		field.Bool("readonly").Default(false),
		field.Bool("requires_confirmation").Default(false),
		field.Bool("supports_streaming").Default(false),
		field.Bool("supports_concurrency").Default(false),
		field.Text("parameters_schema_json").Default("{}"),
		field.Text("result_schema_json").Default("{}"),
		field.Text("config_schema_json").Default("{}"),
		field.Text("config_json").Default("{}"),
		field.Text("fallback_config_json").StorageKey("default_config_json").Default("{}"),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
		// P2-B: tenant isolation. empty = shared/legacy (visible to all workspaces);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").Comment("owning workspace ID; empty = shared/system builtin"),
	}
}

// Indexes of the PlatformTool.
func (PlatformTool) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("category").StorageKey("idx_tools_category"),
		index.Fields("enabled").StorageKey("idx_tools_enabled"),
		index.Fields("deleted_at").StorageKey("idx_tools_deleted_at"),
		index.Fields("enabled", "deleted_at").StorageKey("idx_tools_enabled_deleted"),
		index.Fields("category", "enabled", "deleted_at").StorageKey("idx_tools_cat_enabled_deleted"),
		// P2-B: tenant isolation index — filter tools by workspace visibility.
		index.Fields("workspace_id", "enabled"),
	}
}
