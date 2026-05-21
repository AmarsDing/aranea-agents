package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// FlowLogEvent maps table flow_log_events (FlowLogger v2 persistence).
type FlowLogEvent struct {
	ent.Schema
}

func (FlowLogEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "flow_log_events"},
	}
}

func (FlowLogEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("trace_id").MaxLen(128).Default(""),
		field.String("session_id").MaxLen(128).Default(""),
		field.String("run_id").MaxLen(128).Default(""),
		field.String("team_id").MaxLen(128).Default(""),
		field.String("domain").MaxLen(32).Default(""),
		field.String("agent_key").MaxLen(128).Default(""),
		field.String("step_id").MaxLen(128).Default(""),
		field.String("flow_phase").MaxLen(16).Default(""),
		field.String("severity").MaxLen(16).Default("info"),
		field.String("title").MaxLen(256).Default(""),
		field.Text("message").Default(""),
		field.Text("payload_json").Default("{}"),
		field.Time("created_at"),
	}
}

func (FlowLogEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("trace_id", "created_at"),
		index.Fields("session_id", "created_at"),
		index.Fields("run_id", "created_at"),
	}
}
