package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CodingProject maps table coding_projects：可派发任务的本机项目目录注册表。
// 外部 agent 的 cwd 白名单——用户语音中的项目名经此表解析为磁盘路径。
type CodingProject struct {
	ent.Schema
}

func (CodingProject) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "coding_projects"},
	}
}

func (CodingProject) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64),
		field.String("workspace").Default("default").MaxLen(128),
		// 语音解析键，如 aranea-agents
		field.String("name").MaxLen(256),
		// 本机绝对路径，如 F:\aranea-agents
		field.String("path").MaxLen(1024),
		// 描述（辅助 LLM 消歧）
		field.String("description").Default("").MaxLen(2048),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (CodingProject) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace", "name").Unique(),
	}
}
