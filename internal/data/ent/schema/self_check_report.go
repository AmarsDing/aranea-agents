package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SelfCheckReport maps table self_check_reports for self-check cycle persistence.
type SelfCheckReport struct {
	ent.Schema
}

func (SelfCheckReport) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "self_check_reports"},
	}
}

func (SelfCheckReport) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.Text("check_results_json").Default("[]"),
		field.String("overall_status").MaxLen(16).Default("passed"), // passed / warning / failed
		field.Text("repair_actions_json").Default("[]"),
		field.Time("started_at"),
		field.Time("finished_at"),
		field.Int64("duration_ms").Default(0),
		field.Time("created_at"),
	}
}

func (SelfCheckReport) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("overall_status", "created_at"),
		index.Fields("created_at"),
	}
}
