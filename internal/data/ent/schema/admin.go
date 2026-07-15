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
