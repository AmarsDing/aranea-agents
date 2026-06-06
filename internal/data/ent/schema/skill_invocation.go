package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SkillInvocation maps table skill_invocation (incl. legacy columns from ensureLegacyColumns).
type SkillInvocation struct {
	ent.Schema
}

func (SkillInvocation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "skill_invocation"},
	}
}

func (SkillInvocation) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("skill_id").MaxLen(256),
		field.String("agent_id").Default(""),
		field.String("status").Default("pending"),
		field.Text("input_json").Default("{}"),
		field.Text("output_json").Default("{}"),
		field.Text("error_message").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("skill_version").Default(""),
		field.String("user_id").Default(""),
		field.String("session_id").Default(""),
		field.Int("duration_ms").Default(0),
		field.String("started_at").Default(""),
		field.String("ended_at").Default(""),
		field.Text("input_preview").Default(""),
		field.String("input_hash").Default(""),
		field.Text("output_preview").Default(""),
		field.String("error_code").Default(""),
		field.String("source").Default("runtime").MaxLen(64),
		field.String("activation_id").Default("").MaxLen(256),
		field.String("message_id").Default("").MaxLen(256),
		field.JSON("selection_reason", map[string]any{}).Optional(),
		field.String("outcome").Default("").MaxLen(32),
		field.JSON("token_usage", map[string]any{}).Optional(),
		field.String("deleted_at").Default("").Optional(),
		field.String("analyzed_at").Default("").Optional(),
	}
}

func (SkillInvocation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("skill_id", "created_at").StorageKey("idx_skill_invocation_skill"),
		index.Fields("session_id").StorageKey("idx_skill_invocation_session"),
		index.Fields("agent_id").StorageKey("idx_skill_invocation_agent"),
		index.Fields("deleted_at").StorageKey("idx_skill_invocation_deleted_at"),
	}
}
