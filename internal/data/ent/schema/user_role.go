package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserRole is the user→role 边表（user_id = admins.id）。多角色；为 roles 的权威来源。
type UserRole struct {
	ent.Schema
}

func (UserRole) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_roles"},
	}
}

func (UserRole) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.Int64("user_id").Comment("FK admins.id"),
		field.Int64("role_id").Comment("FK roles.id"),
		field.Time("create_time").Default(time.Now).Immutable(),
	}
}

func (UserRole) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "role_id").Unique(),
		index.Fields("role_id"),
	}
}
