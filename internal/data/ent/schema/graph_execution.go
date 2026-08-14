package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GraphExecution struct {
	ent.Schema
}

func (GraphExecution) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_executions"},
	}
}

func (GraphExecution) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("graph_id").MaxLen(64),
		field.String("session_id").MaxLen(64).Default(""),
		// spirit_session_id: 跨会话聚合键（root spirit session），Y5 持久化——
		// 重启恢复后 resume 仍需正确的会话归属。
		field.String("spirit_session_id").MaxLen(64).Default(""),
		// definition_hash: 执行启动时 GraphBuildConfig 的 SHA256（Y4）——
		// resume 前与当前定义比对，不一致拒绝恢复。
		field.String("definition_hash").MaxLen(64).Default(""),
		field.String("status").MaxLen(32).Default("running"),
		field.String("current_node").MaxLen(128).Default(""),
		field.String("lineage_id").MaxLen(128).Default(""),
		field.Text("error_message").Default(""),
		field.Text("current_state_json").Default("{}"),
		field.Text("steps_json").Default("[]"),
		field.Time("started_at"),
		field.Time("finished_at").Optional().Nillable(),
	}
}

func (GraphExecution) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("graph_id"),
		index.Fields("lineage_id"),
		index.Fields("status"),
		index.Fields("graph_id", "status", "started_at"),
	}
}
