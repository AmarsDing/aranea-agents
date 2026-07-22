package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ResourceAccessAudit holds the schema for the resource_access_audits table (M71).
// Unified audit trail for cross-agent resource access (memberfs / deptmail /
// sessionaccess). Append-only: rows are never updated or deleted.
type ResourceAccessAudit struct {
	ent.Schema
}

func (ResourceAccessAudit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "resource_access_audits"},
	}
}

func (ResourceAccessAudit) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64),
		field.String("actor_agent_id").MaxLen(256).Comment("accessing agent ID"),
		field.String("actor_role").MaxLen(32).Comment("dept_lead | spirit"),
		field.String("action").MaxLen(32).Comment("list_files | read_file | search_files | send_mail | read_mail | reply_mail | search_messages | list_sessions | read_session"),
		field.String("target_agent_id").MaxLen(256).Default("").Comment("target agent ID (optional)"),
		field.String("target_dept_id").MaxLen(256).Default("").Comment("target department ID (optional)"),
		field.String("relation").MaxLen(16).Default("none").Comment("org_home | team_owner | none"),
		field.String("resource_uri").MaxLen(512).Default("").Comment("file path | session_id | message_id"),
		field.String("result").MaxLen(8).Comment("allowed | denied"),
		field.String("deny_reason").MaxLen(256).Default(""),
		field.Time("created_at").Default(timeNow),
	}
}

func (ResourceAccessAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("actor_agent_id", "created_at").StorageKey("idx_raaudit_actor_created"),
		index.Fields("target_agent_id", "created_at").StorageKey("idx_raaudit_target_created"),
	}
}
