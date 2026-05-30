package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Position struct {
	ent.Schema
}

func (Position) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "positions"},
	}
}

func (Position) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("deleted_at"),
		index.Fields("department_key"),
		index.Fields("key", "department_key").Unique(),
	}
}

func (Position) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("key").MaxLen(128),
		field.String("name").MaxLen(256),
		field.String("department_key").MaxLen(128),
		field.Text("description").Default(""),
		field.Text("responsibilities_json").Default("{}"),
		field.Strings("skills_required").Optional(),
		field.String("seniority_level").Default(""),
		field.Int("sort_order").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
