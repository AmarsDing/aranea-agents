package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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

func (Agent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("deleted_at"),
		index.Fields("deleted_at", "status"),
		index.Fields("position_key", "agent_variant").Unique(),
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
		field.String("system_prompt_mode").Default(""),
		field.Int("context_window").Default(0),
		field.Int("budget_monthly_cents").Default(0),
		field.Text("config_json").Default(""),
		field.Text("roles_json").Default("[]"),
		field.String("created_by").Default("").Comment("creator user id from auth context"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
		// CLI-20: system admin agent support.
		field.Bool("readonly").Default(false).Comment("system agents cannot be deleted"),
		field.Enum("kind").Values("user", "system", "system_builtin", "industry_template", "marketplace", "certified").Default("user").Comment("agent kind: user | system | system_builtin | industry_template | marketplace | certified"),
		field.String("position_key").Default("").Comment("FK to positions.key"),
		field.String("taxonomy_position_id").Default("").Comment("FK to taxonomy positions"),
		field.String("agent_variant").Default("general").Comment("variant within position: general/code_review/architect/..."),
		field.Text("variant_description").Default("").Comment("human-readable description of this variant"),
	}
}
