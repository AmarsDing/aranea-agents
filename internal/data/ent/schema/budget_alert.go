package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BudgetAlert maps table budget_alerts.
type BudgetAlert struct {
	ent.Schema
}

func (BudgetAlert) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "budget_alerts"},
	}
}

func (BudgetAlert) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(512),
		field.String("scope_type").MaxLen(64),
		field.String("scope_id").MaxLen(512),
		field.Float("alert_ratio").Default(0.8),
		field.Bool("enabled").Default(true),
		field.String("last_fired_at").Default(""),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (BudgetAlert) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope_type", "scope_id", "alert_ratio").Unique(),
	}
}
