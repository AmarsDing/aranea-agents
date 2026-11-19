package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CredentialExecution maps credential_executions rows（凭据执行审计明细）。
// 记录谁/哪个 agent/何站点/何动作/裁决结果；永不写 secret 明文或派生物。
type CredentialExecution struct {
	ent.Schema
}

func (CredentialExecution) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "credential_executions"},
	}
}

func (CredentialExecution) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.Int64("credential_id").Default(0), // 0 = 裁决发生在凭据定位前（如 site 无凭据）
		field.String("site").MaxLen(256),
		field.String("action").MaxLen(32),     // login_execute | token_issue
		field.String("actor_type").MaxLen(16), // user | agent | system
		field.String("actor_key").Default("").MaxLen(128),
		field.String("agent_id").Default("").MaxLen(128),
		field.String("session_id").Default("").MaxLen(128),
		field.String("verdict").MaxLen(32), // allowed | denied_policy | denied_sensitivity | failed
		field.String("reason").Default("").MaxLen(512),
		field.Time("created_at"),
	}
}

func (CredentialExecution) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("credential_id", "created_at"),
		index.Fields("site", "created_at"),
	}
}
