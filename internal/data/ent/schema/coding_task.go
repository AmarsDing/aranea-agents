package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CodingTask maps table coding_tasks：一次外部编程 agent 任务执行记录。
// status 取值见 biz/agentbridge 任务状态机（dispatched/running/
// awaiting_approval/cancelling/done/failed/cancelled）。
type CodingTask struct {
	ent.Schema
}

func (CodingTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "coding_tasks"},
	}
}

func (CodingTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64),
		field.String("workspace").Default("default").MaxLen(128),
		// 发起任务的精灵会话 ID（审批卡片路由目标）
		field.String("session_id").MaxLen(256),
		field.String("agent_id").MaxLen(64),
		field.String("project_id").MaxLen(64),
		field.Text("prompt").Default(""),
		field.String("status").Default("dispatched").MaxLen(32),
		// ACP 侧 session ID
		field.String("acp_session_id").Default("").MaxLen(256),
		// 完成摘要（截断 4000 字符）
		field.Text("summary").Default(""),
		field.Text("error").Default(""),
		// 进度事件计数（限流统计）
		field.Int("progress_count").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("completed_at").Default(""),
	}
}

func (CodingTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("status"),
	}
}
