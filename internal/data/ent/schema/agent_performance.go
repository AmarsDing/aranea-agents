package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AgentPerformance struct {
	ent.Schema
}

func (AgentPerformance) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "agent_performances"}}
}

func (AgentPerformance) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(256).Immutable().Unique(),
		field.String("agent_key").MaxLen(512).Default(""),
		field.String("task_type").MaxLen(256).Default(""),
		field.Int("total_runs").Default(0),
		field.Int("success_runs").Default(0),
		field.Float("success_rate").Default(0),
		field.Float("avg_dq_score").Default(0),
		field.Int64("avg_duration_ms").Default(0),
		field.String("last_executed_at").Default(""),
	}
}

func (AgentPerformance) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_key", "task_type").Unique(),
	}
}
