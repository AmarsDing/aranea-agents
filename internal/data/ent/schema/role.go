package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Role is the RBAC role entity (三项目整合 P1 权限底座).
// 内置角色对应整合方案 §3.1 九角色；自定义角色 F 期放开增删。
type Role struct {
	ent.Schema
}

func (Role) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "roles"},
	}
}

func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("code").Unique().MaxLen(64).
			Comment("role code: superadmin/sysadmin/secadmin/auditor/dataowner/ceo/oncall/viewer/user"),
		field.String("name").Default("").MaxLen(128),
		field.String("description").Default("").MaxLen(512),
		field.Bool("builtin").Default(false).Comment("builtin roles cannot be deleted"),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Role) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("builtin"),
	}
}
