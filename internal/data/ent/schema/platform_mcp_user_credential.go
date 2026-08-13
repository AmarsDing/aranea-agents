package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformMCPUserCredential maps table mcp_server_user_credential.
type PlatformMCPUserCredential struct {
	ent.Schema
}

func (PlatformMCPUserCredential) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mcp_server_user_credential"},
	}
}

func (PlatformMCPUserCredential) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("mcp_server_id").MaxLen(256).NotEmpty(),
		field.String("user_id").MaxLen(256).NotEmpty(),
		field.String("credential_key").MaxLen(512).NotEmpty().Sensitive(),
		field.String("status").Default("active"),
		field.String("secret_ref").Default("").Sensitive(),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (PlatformMCPUserCredential) Indexes() []ent.Index {
	return []ent.Index{
		// 软删除感知的复合唯一索引：仅约束活跃行，墓碑不再阻塞同 key 重建。
		// 谓词须写 PG 规范形式（''::text），Atlas diff 为纯文本比对。
		index.Fields("mcp_server_id", "user_id", "credential_key").Unique().
			StorageKey("idx_mcp_credential_unique_active").
			Annotations(entsql.IndexWhere("deleted_at = ''::text")),
	}
}
