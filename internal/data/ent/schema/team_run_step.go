package schema

import (
	"aranea-agents/internal/biz"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// TeamRunStep maps legacy table team_run_steps.
type TeamRunStep struct {
	ent.Schema
}

func (TeamRunStep) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_run_steps"},
	}
}

func (TeamRunStep) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("run_id").MaxLen(256),
		field.String("team_id").MaxLen(256),
		field.String("agent_id").Default(""),
		field.String("agent_key").Default(""),
		field.String("agent_name").Default(""),
		field.String("role").Default(""),
		field.Int("sort_order").Default(0),
		field.String("status").Default(biz.TeamMemberStepStatusOK),
		field.Text("input_preview").Default(""),
		field.Text("output_preview").Default(""),
		field.Int("token_in").Default(0),
		field.Int("token_out").Default(0),
		field.Int64("cost_micro_usd").Default(0),
		field.Int("duration_ms").Default(0),
		field.Text("error_message").Default(""),
		field.String("started_at").Default(""),
		field.String("finished_at").Default(""),
		field.String("created_at").Default(""),
		field.Int("tool_call_count").Default(0),
	}
}
