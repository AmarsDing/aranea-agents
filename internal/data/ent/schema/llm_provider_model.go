package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// LlmProviderModel maps table llm_provider_models (legacy platform "llm-provider-models").
type LlmProviderModel struct {
	ent.Schema
}

func (LlmProviderModel) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "llm_provider_models"},
	}
}

func (LlmProviderModel) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("model_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.String("provider").Default(""),
		field.String("model").Default(""),
		field.Text("config_json").Default(""),
		field.Text("metadata_json").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
