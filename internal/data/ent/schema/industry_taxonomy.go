package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type IndustryTaxonomy struct {
	ent.Schema
}

func (IndustryTaxonomy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "industry_taxonomy"},
	}
}

func (IndustryTaxonomy) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("taxonomy_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.String("parent_id").Default(""),
		field.String("level").Default(""),
		field.String("scenario_key").Default(""),
		field.String("workspace_id").Default(""),
		field.String("owner_user_id").Default(""),
		field.Bool("is_system").Default(false),
		field.Text("config_json").Default(""),
		field.Text("metadata_json").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (IndustryTaxonomy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("parent_id", "sort_order").StorageKey("idx_taxonomy_parent"),
		index.Fields("level", "sort_order").StorageKey("idx_taxonomy_level"),
	}
}
