package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DecisionRecord is one entry of the unified decision record layer (M80).
// Table is created by DDL migration 20261250; this schema documents the contract for
// Ent codegen. Runtime access uses the raw-SQL repo (internal/data decision repo).
type DecisionRecord struct {
	ent.Schema
}

func (DecisionRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "decision_records"},
	}
}

func (DecisionRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("decision_key").MaxLen(36).NotEmpty().Unique().Immutable(),
		field.String("category").MaxLen(32).Default(""),
		field.Text("scenario").Default(""),
		field.Text("reasoning").Default(""),
		field.String("outcome").MaxLen(64).Default(""),
		field.Float("confidence").Optional().Nillable(),
		field.String("actor_type").MaxLen(16).Default(""),
		field.String("actor_key").MaxLen(128).Default(""),
		field.Int64("parent_decision_id").Optional().Nillable(),
		field.Text("related_entities").Default("[]"),
		field.Text("source_ref").Default("{}"),
		field.Text("metadata").Default("{}"),
		field.String("workspace_id").MaxLen(64).Default(""),
		field.String("created_at").MaxLen(40).Default(""),
		field.String("updated_at").MaxLen(40).Default(""),
	}
}

func (DecisionRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("category", "created_at").StorageKey("idx_decision_records_category_created"),
		index.Fields("actor_key").StorageKey("idx_decision_records_actor"),
		index.Fields("parent_decision_id").StorageKey("idx_decision_records_parent"),
		index.Fields("workspace_id", "category").StorageKey("idx_decision_records_ws_category"),
	}
}
