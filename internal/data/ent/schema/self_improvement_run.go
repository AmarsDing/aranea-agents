package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

// SelfImprovementRun maps table self_improvement_runs for the platform
// self-improvement (V3) seven-stage execution loop instance.
type SelfImprovementRun struct {
	ent.Schema
}

func (SelfImprovementRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "self_improvement_runs"},
	}
}

func (SelfImprovementRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable().DefaultFunc(uuid.NewString),
		field.String("suggestion_id").MaxLen(64).NotEmpty().Unique(), // 1:1 unified_evolution_suggestions
		field.String("status").MaxLen(24).NotEmpty().Default("detected"),
		field.String("trigger_source").MaxLen(32).NotEmpty(),
		field.String("patch_kind").MaxLen(16).Optional().Default(""),
		field.String("risk_level").MaxLen(8).Optional().Default(""),
		field.String("base_ref").MaxLen(64).Optional().Default(""),
		field.String("branch").MaxLen(128).Optional().Default(""),
		field.String("worktree_path").MaxLen(256).Optional().Default(""),
		field.Text("diff").Optional(),
		field.JSON("diff_stats", map[string]int{}).Optional(),
		field.JSON("diagnosis", map[string]any{}).Optional(),
		field.JSON("verification_report", []map[string]any{}).Optional(),
		field.JSON("critic_report", map[string]any{}).Optional(),
		field.JSON("governance", map[string]any{}).Optional(),
		field.Int("attempts").Default(0),
		field.String("approved_by").MaxLen(64).Optional().Default(""),
		field.String("applied_commit").MaxLen(64).Optional().Default(""),
		field.String("rollback_pointer").MaxLen(64).Optional().Default(""),
		field.Time("observe_until").Optional().Nillable(),
		field.String("closed_reason").MaxLen(64).Optional().Default(""),
		field.JSON("metadata", map[string]any{}).Optional(),
		field.Time("created_at").Immutable().Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (SelfImprovementRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("trigger_source", "created_at"),
		index.Fields("observe_until"),
	}
}
