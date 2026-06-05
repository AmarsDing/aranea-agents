package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// PlatformSkill maps table skill (Ent type name avoids collisions).
type PlatformSkill struct {
	ent.Schema
}

func (PlatformSkill) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "skill"},
	}
}

func (PlatformSkill) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("skill_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.Text("config_json").Default("{}"),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
		field.String("parent_id").Default(""),
		field.String("level").Default(""),
		field.String("agent_id").Default(""),
		field.String("provider").Default(""),
		field.String("model").Default(""),
		field.String("kind").Default("markdown").MaxLen(64),
		field.String("risk_level").Default("low").MaxLen(32),
		field.String("entry_path").Default("SKILL.md").MaxLen(512),
		field.Bool("filesystem_missing").Default(false),
		field.String("visibility").Default("workspace").MaxLen(64),
		field.Text("fallback_config_json").StorageKey("default_config_json").Default("{}"),
		field.String("parent_version_id").Default("").MaxLen(256),
		field.Text("evolution_reason").Default(""),
		field.String("lifecycle_status").Default("active").MaxLen(64),
	}
}
