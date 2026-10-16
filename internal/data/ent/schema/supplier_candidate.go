package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SupplierCandidate is MVP media/commerce domain table `supplier_candidates`.
type SupplierCandidate struct {
	ent.Schema
}

func (SupplierCandidate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "supplier_candidates"},
	}
}

func (SupplierCandidate) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),

		field.String("opportunity_id").Default("").MaxLen(256),
		field.String("name").MaxLen(512),
		field.String("url").Default("").MaxLen(2048),
		field.String("price_range").Default("").MaxLen(256),
		field.Text("notes").Default(""),
		field.String("status").Default("suggested").MaxLen(64),

		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (SupplierCandidate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
