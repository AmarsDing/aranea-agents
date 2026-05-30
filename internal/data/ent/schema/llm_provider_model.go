package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
		field.Bool("capability_text").Default(false),
		field.Bool("capability_vision").Default(false),
		field.Bool("capability_audio").Default(false),
		field.Bool("capability_file").Default(false),
		field.Bool("capability_tool_call").Default(false),
		field.Bool("capability_cache").Default(false),
		field.Bool("capability_thinking").Default(false),
		field.Bool("capability_text_only").Default(false),
		field.Bool("capabilities_explicit").Default(false),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (LlmProviderModel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "enabled", "sort_order").StorageKey("idx_provider_models_provider"),
	}
}
