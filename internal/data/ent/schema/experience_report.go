package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExperienceReport maps table experience_reports.
type ExperienceReport struct {
	ent.Schema
}

func (ExperienceReport) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "experience_reports"},
	}
}

func (ExperienceReport) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("tenant_id").Default("").MaxLen(256),
		field.String("session_id").Default("").MaxLen(256),
		field.String("invocation_id").Default("").MaxLen(256),
		field.String("skill_id").Default("").MaxLen(256),
		field.String("skill_name").Default("").MaxLen(256).Optional(),
		field.Bool("is_success").Default(false),
		field.Int("score").Default(0),
		field.JSON("failure_tags", []string{}).Optional(),
		field.Text("flow_summary").Default(""),
		field.Text("optimization_advice").Default(""),
		field.JSON("selection_snapshot", map[string]any{}).Optional(),
		field.Text("root_cause_analysis").Default(""),
		field.Text("suggested_fix").Default(""),
		field.String("generated_suggestion_id").Default("").MaxLen(256),
		field.String("created_at").Default(""),
	}
}

func (ExperienceReport) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("skill_id", "created_at").StorageKey("idx_experience_report_skill_time"),
		index.Fields("invocation_id").StorageKey("idx_experience_report_invocation"),
	}
}
