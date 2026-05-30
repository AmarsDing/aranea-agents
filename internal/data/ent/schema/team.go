package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Team maps legacy table teams.
type Team struct {
	ent.Schema
}

func (Team) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "teams"},
	}
}

func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("team_key").Unique().MaxLen(512),
		field.String("display_name").MaxLen(1024),
		field.String("status").Default("active"),
		field.Bool("is_default").Default(false),
		field.Text("definition_json").Default(""),
		field.String("adk_app_name").Default(""),
		field.String("category_industry_id").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
