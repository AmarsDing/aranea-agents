package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Organization maps the organizations table (renamed from industry_taxonomy).
// Represents the company organizational structure: company → department → position.
type Organization struct {
	ent.Schema
}

func (Organization) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "organizations"},
	}
}

func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("org_key").Unique().MaxLen(512),    // renamed from taxonomy_key
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.String("parent_id").Default(""),
		field.String("level").Default(""),               // "company" | "department" | "position"
		field.String("scenario_key").Default(""),
		field.String("workspace_id").Default(""),
		field.String("owner_user_id").Default(""),
		field.Bool("is_system").Default(false),
		field.Text("config_json").Default(""),
		field.Text("metadata_json").Default(""),
		// department-level fields
		field.String("dept_lead_agent_id").Default("").Optional().Comment("department lead Agent ID (only for department level nodes)"),
		field.Text("dept_lead_config_json").Default("{}").Comment("department lead config overrides (prompt, tools, etc.)"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}

func (Organization) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("parent_id", "sort_order").StorageKey("idx_org_parent"),
		index.Fields("level", "sort_order").StorageKey("idx_org_level"),
	}
}
