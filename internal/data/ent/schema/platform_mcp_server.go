package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformMCPServer maps table mcp_server (legacy platform "mcp-servers").
type PlatformMCPServer struct {
	ent.Schema
}

func (PlatformMCPServer) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mcp_server"},
	}
}

func (PlatformMCPServer) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("server_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.Text("config_json").Default("").Sensitive().Comment("MCP connection JSON; secrets encrypted at rest (C-05)"),
		field.Text("metadata_json").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
		// P2-B: tenant isolation. empty = shared/legacy (visible to all workspaces);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").Comment("owning workspace ID; empty = shared/system builtin"),
	}
}

func (PlatformMCPServer) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "enabled").StorageKey("idx_mcp_server_status_enabled"),
		index.Fields("deleted_at").StorageKey("idx_mcp_server_deleted_at"),
		// P2-B: tenant isolation index — filter mcp servers by workspace visibility.
		index.Fields("workspace_id", "enabled"),
	}
}
