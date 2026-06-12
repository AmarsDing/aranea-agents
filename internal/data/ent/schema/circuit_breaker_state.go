package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CircuitBreakerState maps table circuit_breaker_states for persisting circuit breaker runtime state.
type CircuitBreakerState struct {
	ent.Schema
}

func (CircuitBreakerState) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "circuit_breaker_states"},
	}
}

func (CircuitBreakerState) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").MaxLen(256).Unique().NotEmpty(), // tool name
		field.String("state").MaxLen(16).Default("closed"),  // closed | open | half_open
		field.Int("failure_count").Default(0),
		field.Int("success_count").Default(0),
		field.Time("last_failure_time").Optional().Nillable(),
		field.Time("last_state_change").Optional().Nillable(),
		field.Time("updated_at"),
	}
}

func (CircuitBreakerState) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("state"),
	}
}
