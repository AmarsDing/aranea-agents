package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ModelTokenUsageHourly maps table model_token_usage_hourly.
type ModelTokenUsageHourly struct {
	ent.Schema
}

func (ModelTokenUsageHourly) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "model_token_usage_hourly"},
	}
}

func (ModelTokenUsageHourly) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(512),
		field.String("hour_key").MaxLen(32),
		field.String("workspace_id").Default("").MaxLen(512),
		field.String("agent_id").Default("").MaxLen(512),
		field.String("agent_key").Default("").MaxLen(512),
		field.String("provider_code").Default("").MaxLen(256),
		field.String("model_api_id").Default("").MaxLen(512),
		field.String("usage_kind").Default("chat").MaxLen(64),
		field.Int("call_count").Default(0),
		field.Int("request_count").Default(0),
		field.Int("success_count").Default(0),
		field.Int("failed_count").Default(0),
		field.Int("cancelled_count").Default(0),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.Int("cached_input_tokens").Default(0),
		field.Int("reasoning_tokens").Default(0),
		field.Int("embedding_tokens").Default(0),
		field.Int("total_tokens").Default(0),
		field.Int64("total_cost_micro_usd").Default(0),
		field.Float("avg_latency_ms").Default(0),
		field.Float("avg_tokens_per_second").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (ModelTokenUsageHourly) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("hour_key", "workspace_id", "agent_id", "provider_code", "model_api_id", "usage_kind").Unique(),
		index.Fields("hour_key"),
	}
}
