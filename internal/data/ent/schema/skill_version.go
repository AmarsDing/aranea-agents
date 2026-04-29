package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// SkillVersion maps table skill_version.
type SkillVersion struct {
	ent.Schema
}

func (SkillVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "skill_version"},
	}
}

func (SkillVersion) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("skill_id").MaxLen(256),
		field.String("version").MaxLen(64),
		field.String("status").Default("active"),
		field.Text("content_markdown").Default(""),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
