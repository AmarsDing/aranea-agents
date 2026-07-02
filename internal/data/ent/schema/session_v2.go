package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionV2 是 spirit 会话根 entity（v2 模型）。
type SessionV2 struct {
	ent.Schema
}

func (SessionV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sessions_v2"},
	}
}

func (SessionV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(128).Unique().Immutable(),
		field.String("user_id").MaxLen(128).Default(""),
		field.String("spirit_agent_id").MaxLen(128).Default(""),
		field.String("status").MaxLen(32).Default("active"),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow),
	}
}

func (SessionV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status").StorageKey("idx_sessions_v2_user_status"),
	}
}
