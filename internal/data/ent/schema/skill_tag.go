package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SkillTag maps table skill_tags：技能标签字典（治理 + 预建）。
// 使用计数不落库，List 时实时聚合 skill.metadata_json。
type SkillTag struct {
	ent.Schema
}

func (SkillTag) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "skill_tags"},
	}
}

func (SkillTag) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		// 规范标签 token，如 file_type:xlsx / domain:sales / figma。
		field.String("name").Unique().MaxLen(256),
		// `:` 前缀维度（file_type/domain），无维度为空串，UI 分组用。
		field.String("dimension").Default("").MaxLen(128),
		// system | user
		field.String("source").Default("user").MaxLen(32),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (SkillTag) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("dimension"),
	}
}
