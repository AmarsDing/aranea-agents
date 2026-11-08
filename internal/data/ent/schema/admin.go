package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Admin holds the schema definition for the Admin entity.
type Admin struct {
	ent.Schema
}

// Fields of the Admin.
func (Admin) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("name").Default(""),
		field.String("email").Default(""),
		field.String("avatar").Default(""),
		field.String("access").Default(""), // TECH-DEBT(DB-N8): evaluate if 'access' field should be .Sensitive() — pending business review
		field.String("password").Default("").Sensitive(),
		// B-01 / P2-A: 1:1 admin → workspace binding. Empty treated as default
		// via JWT EffectiveWorkspaceID; migration 20261006 backfills existing rows.
		field.String("workspace_id").Default("default").MaxLen(128),
		// P1 权限底座（三项目整合）：角色 / 密级 / 部门 / 状态 / 强制改密 / 会话吊销版本。
		// 逻辑名为 User；物理表 admins 暂保留（改名 blast radius 大，另立任务）。
		field.JSON("roles", []string{}).Default([]string{}).
			Comment("role codes; source of truth is user_roles edge, this JSON is a read cache"),
		field.Int("clearance").Default(1).Range(1, 5).
			Comment("密级资质 L1-L5（1 最低，5 最高）"),
		field.String("dept").Default("").
			Comment("department code"),
		field.Enum("status").Values("active", "frozen", "offboarded").Default("active").
			Comment("account status"),
		field.Bool("force_reset").Default(false).
			Comment("require password change on next login (wb migrated users)"),
		field.Int64("token_version").Default(0).
			Comment("session revocation version; bumped on kick/password-change/offboard/role-change"),
		field.Time("create_time").Default(time.Now).Immutable(),
		field.Time("update_time").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the Admin.
func (Admin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").StorageKey("idx_admins_name"),
		index.Fields("email").StorageKey("idx_admins_email"),
		index.Fields("workspace_id").StorageKey("idx_admins_workspace_id"),
	}
}
