package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformChannel maps SQLite table `channel`.
type PlatformChannel struct {
	ent.Schema
}

func (PlatformChannel) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel"},
	}
}

func (PlatformChannel) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("channel_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.Text("config_json").Default("{}"),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
		// P2-B: tenant isolation. empty = shared/legacy (visible to all workspaces);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").Comment("owning workspace ID; empty = shared/system builtin"),
	}
}

func (PlatformChannel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "enabled").StorageKey("idx_channel_status_enabled"),
		index.Fields("deleted_at").StorageKey("idx_channel_deleted_at"),
		// P2-B: tenant isolation index — filter channels by workspace visibility.
		index.Fields("workspace_id", "enabled"),
	}
}
