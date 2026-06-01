package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ModelPricingRule maps table model_pricing_rules.
type ModelPricingRule struct {
	ent.Schema
}

func (ModelPricingRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "model_pricing_rules"},
	}
}

func (ModelPricingRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(512),
		field.String("provider_code").MaxLen(256),
		field.String("model_api_id").MaxLen(512),
		field.String("currency").Default("USD"),
		field.Int64("input_price_micro_usd_per_1k").Default(0),
		field.Int64("output_price_micro_usd_per_1k").Default(0),
		field.Int64("cached_input_price_micro_usd_per_1k").Default(0),
		field.Int64("reasoning_price_micro_usd_per_1k").Default(0),
		field.Int64("embedding_price_micro_usd_per_1k").Default(0),
		field.Int64("cache_write_price_micro_usd_per_1k").Default(0),
		field.Float("input_price_usd_per_1m").Default(0),
		field.Float("output_price_usd_per_1m").Default(0),
		field.Float("cached_input_price_usd_per_1m").Default(0),
		field.Float("reasoning_price_usd_per_1m").Default(0),
		field.Float("embedding_price_usd_per_1m").Default(0),
		field.Float("cache_write_price_usd_per_1m").Default(0),
		field.String("effective_from").Default(""),
		field.String("effective_to").Default(""),
		field.Bool("is_active").Default(true),
		field.String("source").Default("manual"),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (ModelPricingRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_code", "model_api_id", "effective_from").Unique(),
		index.Fields("provider_code", "model_api_id", "is_active", "effective_from").StorageKey("idx_pricing_rules_model_active"),
	}
}
