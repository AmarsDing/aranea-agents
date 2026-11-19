package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Permission is the二级权限码 entity (menu | api)。见 P0 详设《01-权限模型详设》权限码清单。
type Permission struct {
	ent.Schema
}

func (Permission) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "permissions"},
	}
}

func (Permission) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique().Immutable(),
		field.String("code").Unique().MaxLen(128).
			Comment("permission code, e.g. knowledge.read / menu.knowledge"),
		field.Enum("kind").Values("menu", "api").Default("api").
			Comment("menu = front-end visibility only; api = back-end enforcement"),
		field.String("description").Default("").MaxLen(512),
		field.Time("create_time").Default(time.Now).Immutable(),
	}
}

func (Permission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kind"),
	}
}
