package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CodingAgent maps table coding_agents：外部编程 CLI Agent 注册表。
// command+args 描述 ACP stdio 子进程启动方式（如 codebuddy --acp /
// npx -y @zed-industries/claude-code-acp）。
type CodingAgent struct {
	ent.Schema
}

func (CodingAgent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "coding_agents"},
	}
}

func (CodingAgent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64),
		field.String("workspace").Default("default").MaxLen(128),
		// 唯一标识：claude_code / codex / codebuddy
		field.String("agent_key").MaxLen(64),
		field.String("display_name").MaxLen(128),
		// 启动命令（exec.LookPath 解析）
		field.String("command").MaxLen(512),
		field.JSON("args", []string{}).Optional(),
		field.JSON("env", map[string]string{}).Optional(),
		field.Bool("enabled").Default(true),
		// 最近一次派发前探测结果
		field.Bool("last_probe_ok").Default(false),
		field.String("last_probe_error").Default("").MaxLen(2048),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (CodingAgent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace", "agent_key").Unique(),
	}
}
