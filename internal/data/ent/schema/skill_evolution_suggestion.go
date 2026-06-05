package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SkillEvolutionSuggestion maps table skill_evolution_suggestions.
type SkillEvolutionSuggestion struct {
	ent.Schema
}

func (SkillEvolutionSuggestion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "skill_evolution_suggestions"},
	}
}

func (SkillEvolutionSuggestion) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("skill_id").Default("").MaxLen(256),
		field.String("type").Default("").MaxLen(64),
		field.String("status").Default("pending").MaxLen(64),
		field.JSON("source_report_ids", []string{}).Optional(),
		field.Text("trigger_reason").Default(""),
		field.Text("draft_skill_body").Default(""),
		field.String("draft_version_id").Default("").MaxLen(256),
		field.Bool("sandbox_passed").Default(false),
		field.JSON("sandbox_result", map[string]any{}).Optional(),
		field.JSON("pre_verify_result", map[string]any{}).Optional(),
		field.String("approved_by").Default("").MaxLen(256),
		field.String("rejected_by").Default("").MaxLen(256),
		field.Text("rejection_reason").Default(""),
		field.String("created_at").Default(""),
		field.String("resolved_at").Default("").Optional(),
	}
}

func (SkillEvolutionSuggestion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("skill_id", "status").StorageKey("idx_evo_suggestion_skill_status"),
		index.Fields("status", "created_at").StorageKey("idx_evo_suggestion_status_time"),
	}
}
