package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ConfigGraphEdge is one reference edge of the config-asset dependency graph (M81).
// Table is created by DDL migration 20261260; this schema documents the contract for
// Ent codegen. Runtime access uses the raw-SQL repo in internal/data/configgraph.
type ConfigGraphEdge struct {
	ent.Schema
}

func (ConfigGraphEdge) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "config_graph_edges"},
	}
}

func (ConfigGraphEdge) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("src_id").MaxLen(64).Default(""),
		field.String("dst_id").MaxLen(64).Default(""),
		field.String("edge_type").MaxLen(32).NotEmpty(),
		field.String("evidence_json").Default("{}"),
		field.String("workspace_id").MaxLen(64).Default(""),
		field.Int64("generation").Default(0),
		field.String("created_at").MaxLen(40).Default(""),
	}
}

func (ConfigGraphEdge) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("src_id", "dst_id", "edge_type", "generation").Unique(),
		index.Fields("src_id", "generation"),
		index.Fields("dst_id", "generation"),
	}
}
