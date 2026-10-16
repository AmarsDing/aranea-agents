package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Opportunity is MVP media/commerce domain table `opportunities`.
type Opportunity struct {
	ent.Schema
}

func (Opportunity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "opportunities"},
	}
}

func (Opportunity) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),

		field.String("title").MaxLen(1024),
		field.Text("summary").Default(""),
		field.Text("entities_json").Default("[]"),
		field.Text("signals_json").Default("[]"),
		field.String("audience").Default("both").MaxLen(64),
		field.Float("score").Default(0),
		field.String("status").Default("open").MaxLen(64),

		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (Opportunity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
