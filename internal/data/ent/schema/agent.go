package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Agent maps legacy table agents (read paths + counts only until full agent domain is migrated).
type Agent struct {
	ent.Schema
}

func (Agent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agents"},
	}
}

func (Agent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("agent_key").Unique().MaxLen(512),
		field.String("display_name").MaxLen(1024),
		field.String("provider"),
		field.String("model"),
		field.String("status").Default("active"),
		field.Bool("is_default").Default(false),
		field.Bool("is_favorite").Default(false),
		field.String("icon").Default(""),
		field.Text("agent_description").Default(""),
		field.String("category_position_id").Default(""),
		field.String("system_prompt_mode").Default(""),
		field.Int("context_window").Default(0),
		field.Int("budget_monthly_cents").Default(0),
		field.Text("config_json").Default(""),
		field.Text("roles_json").Default("[]"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
