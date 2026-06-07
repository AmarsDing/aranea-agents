package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type GraphDefinition struct {
	ent.Schema
}

func (GraphDefinition) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_definitions"},
	}
}

func (GraphDefinition) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("name").MaxLen(256),
		field.Text("description").Default(""),
		field.Text("state_fields").Default("[]"),
		field.Text("nodes").Default("[]"),
		field.Text("edges").Default("[]"),
		field.Text("conditional_edges").Default("[]"),
		field.Text("subgraphs").Default("[]"),
		field.String("entry_point").MaxLen(128),
		field.String("finish_point").MaxLen(128).Default(""),
		field.Bool("enable_checkpoint").Default(false),
		field.String("execution_engine").MaxLen(16).Default("bsp"),
		field.Text("interrupt_before").Default("[]"),
		field.Text("interrupt_after").Default("[]"),
		field.Text("metadata").Default("{}"),
		field.Int("sort_order").Default(0),
		field.Time("created_at"),
		field.Time("updated_at"),
		// organization redesign: team ownership + verification gates
		field.String("team_id").Default("").Optional().Comment("owning team ID (empty for template graphs)"),
		field.Bool("is_template").Default(false).Comment("whether this graph is a reusable template"),
		field.Text("verification_gates").Default("[]").Comment("verification gate definitions JSON"),
	}
}
