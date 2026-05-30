package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Industry struct {
	ent.Schema
}

func (Industry) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "industries"},
	}
}

func (Industry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("deleted_at"),
		index.Fields("scenario_key"),
		index.Fields("deleted_at", "enabled"),
	}
}

func (Industry) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("key").Unique().MaxLen(128),
		field.String("name").MaxLen(256),
		field.String("icon").Default(""),
		field.Text("description").Default(""),
		field.String("scenario_key").Default(""),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
