package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Credential maps credentials rows（E 阶段凭据模块，需求文档 §3，拍板 V-1/V-2/V-3）。
//
// 硬规则：明文三不进（日志/prompt/API 响应）；本表只有 secret_enc
// （AES-GCM 密文，biz/credential_crypto 设施），无任何明文列。
type Credential struct {
	ent.Schema
}

func (Credential) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "credentials"},
	}
}

func (Credential) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("workspace_id").Default("").MaxLen(64),
		field.String("name").MaxLen(128),
		field.String("site").MaxLen(256), // 策略裁决键，如 github.com
		field.String("url").Default("").MaxLen(512),
		field.String("username").Default("").MaxLen(256),
		field.String("secret_kind").MaxLen(16),                   // password | token | cookie
		field.String("sensitivity").Default("normal").MaxLen(16), // normal | high
		field.String("note").Default("").MaxLen(1024),
		field.Text("secret_enc"), // AES-GCM 密文（base64），永不落明文
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (Credential) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "site"),
		index.Fields("site"),
	}
}
