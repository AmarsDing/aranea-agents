package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CeoboardAudit maps ceoboard_audit rows
// (整合 P2，自 ceo-dashboard SQLite audit 表).
//
// 高危操作前门语义：intent 先落库（ok=false），写失败即拒绝执行（fail-closed）。
// 截断口径（biz 层）：actor 128 / action 64 / target 256 / detail 1024。
type CeoboardAudit struct {
	ent.Schema
}

func (CeoboardAudit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ceoboard_audit"},
	}
}

func (CeoboardAudit) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.Time("at"),
		field.String("actor").Default("").MaxLen(128),
		field.String("action").MaxLen(64),
		field.String("target").Default("").MaxLen(256),
		field.Bool("ok").Default(false),
		field.String("detail").Default("").MaxLen(1024),
	}
}

func (CeoboardAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("at"),
	}
}
