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
		field.Text("config_json").Default(""),
		field.Text("metadata_json").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (PlatformMCPServer) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "enabled").StorageKey("idx_mcp_server_status_enabled"),
		index.Fields("deleted_at").StorageKey("idx_mcp_server_deleted_at"),
	}
}
