package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SkillImportJob maps table skill_import_jobs.
// Persists skill ZIP import state so that in-progress imports survive
// server restarts and are visible across multiple instances.
type SkillImportJob struct {
	ent.Schema
}

func (SkillImportJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "skill_import_jobs"},
	}
}

func (SkillImportJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("status").Default("processing").MaxLen(64),   // processing, completed, failed, applied
		field.String("validation_status").Default("pass").MaxLen(64), // pass, warn, block
		field.String("storage_root").Default("").MaxLen(512),
		field.Text("message").Default("").Optional(),
		field.JSON("candidates_json", map[string]any{}).Optional(),   // serialized []SkillImportCandidate
		field.JSON("conflict_groups_json", map[string]any{}).Optional(), // serialized []SkillConflictGroup
		field.String("temp_dir").Default("").MaxLen(512).Optional(),  // path to temp directory for file content
		field.String("created_at").Default(""),
		field.String("applied_at").Default("").Optional(),
	}
}

func (SkillImportJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at").StorageKey("idx_skill_import_job_status_time"),
	}
}
