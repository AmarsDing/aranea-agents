package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ConfigGraphNode is one node of the config-asset dependency graph (M81).
// Table is created by DDL migration 20261260; this schema documents the contract for
// Ent codegen. Runtime access uses the raw-SQL repo in internal/data/configgraph.
type ConfigGraphNode struct {
	ent.Schema
}

func (ConfigGraphNode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "config_graph_nodes"},
	}
}

func (ConfigGraphNode) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("node_type").MaxLen(32).NotEmpty(),
		field.String("ref_id").MaxLen(128).Default(""),
		field.String("node_key").MaxLen(256).Default(""),
		field.String("display_name").MaxLen(256).Default(""),
		field.String("workspace_id").MaxLen(64).Default(""),
		field.String("status").MaxLen(16).Default("active"),
		field.String("attrs_json").Default("{}"),
		field.Int64("generation").Default(0),
		field.String("created_at").MaxLen(40).Default(""),
		field.String("updated_at").MaxLen(40).Default(""),
	}
}

func (ConfigGraphNode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("node_type", "ref_id", "generation").Unique(),
		index.Fields("node_type", "status", "generation"),
		index.Fields("node_key"),
		index.Fields("workspace_id"),
	}
}
