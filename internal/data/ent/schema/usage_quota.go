package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UsageQuota maps table usage_quotas.
type UsageQuota struct {
	ent.Schema
}

func (UsageQuota) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "usage_quotas"},
	}
}

func (UsageQuota) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(512),
		field.String("scope_type").MaxLen(64),
		field.String("scope_id").MaxLen(512),
		field.Int64("monthly_micro_usd").Default(0),
		field.String("period_start").Default(""),
		field.String("period_end").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (UsageQuota) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope_type", "scope_id").Unique(),
	}
}
