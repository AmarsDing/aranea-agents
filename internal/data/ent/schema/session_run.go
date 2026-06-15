package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionRun maps legacy table session_runs.
type SessionRun struct {
	ent.Schema
}

func (SessionRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "session_runs"},
	}
}

func (SessionRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("phase", "finished_at"),
	}
}

func (SessionRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("session_id").MaxLen(256),
		field.String("turn_id").Default(""),
		field.String("runtime_run_id").Default(""),
		field.String("source").Default(""),
		field.String("phase").Default("interactive"),
		// Deprecated: budget mechanism removed
		field.Int("soft_budget_sec").Default(0),
		// Deprecated: budget mechanism removed
		field.Int("hard_budget_sec").Default(0),
		field.String("checkpoint_id").Default(""),
		field.String("workflow_job_id").Default(""),
		field.String("agent_id").Default(""),
		field.Text("error_message").Default(""),
		field.String("started_at").Default(""),
		field.String("phase_changed_at").Default(""),
		field.String("finished_at").Default(""),
		field.String("resume_started_at").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
