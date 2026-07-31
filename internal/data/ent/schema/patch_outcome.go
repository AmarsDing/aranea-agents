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

// PatchOutcome maps table patch_outcomes for terminal attribution of
// self-improvement runs (V3 Learn stage).
type PatchOutcome struct {
	ent.Schema
}

func (PatchOutcome) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "patch_outcomes"},
	}
}

func (PatchOutcome) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable().DefaultFunc(uuid.NewString),
		field.String("run_id").MaxLen(64).NotEmpty(),
		field.String("suggestion_id").MaxLen(64).NotEmpty(),
		field.String("verdict").MaxLen(16).NotEmpty(), // effective / neutral / regressed
		field.JSON("metrics_before", map[string]any{}).Optional(),
		field.JSON("metrics_after", map[string]any{}).Optional(),
		field.String("rollback_reason").MaxLen(256).Optional().Default(""),
		field.String("pattern_hash").MaxLen(32).Optional().Default(""),
		field.Time("created_at").Immutable().Default(time.Now),
	}
}

func (PatchOutcome) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_id"),
		index.Fields("verdict", "created_at"),
	}
}
