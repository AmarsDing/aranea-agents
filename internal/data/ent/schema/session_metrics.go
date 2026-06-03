package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionMetrics maps table session_metrics — 高频更新的 metrics 字段拆分表.
type SessionMetrics struct {
	ent.Schema
}

func (SessionMetrics) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "session_metrics"},
	}
}

func (SessionMetrics) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id").StorageKey("idx_session_metrics_session"),
	}
}

func (SessionMetrics) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("session_id").Unique().Immutable().MaxLen(256),
		field.Int("message_count").Default(0),
		field.Int("run_count").Default(0),
		field.Int("model_call_count").Default(0),
		field.Int("tool_call_count").Default(0),
		field.Int("skill_call_count").Default(0),
		field.Int("mcp_call_count").Default(0),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.Int("total_tokens").Default(0),
		field.Int64("total_cost_micro_usd").Default(0),
		field.Float("avg_latency_ms").Default(0.0),
		field.Int("error_count").Default(0),
		field.Int("context_used_tokens").Default(0),
		field.Float("context_used_ratio").Default(0.0),
		field.Float("max_context_used_ratio").Default(0.0),
		field.String("context_status").Default(""),
		field.String("last_message_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
