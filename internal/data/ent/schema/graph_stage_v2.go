package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GraphStageV2 是 Graph 流程图（v2 模型），与 PlanBoard 一对一关联。
// 替代 v1 ActivityKindGraphStage（通过 activity.bridge 桥接到 v2）。
//
// 设计：见 docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
// §3.2.3 graph_stages 表 / §3.7.5 GraphStageBlock 与 PlanDAG 的关系
//
// GraphStage.Nodes 是 in-memory 字段（从 graph_nodes 表加载），不在此表持久化。
type GraphStageV2 struct {
	ent.Schema
}

func (GraphStageV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_stages_v2"},
	}
}

func (GraphStageV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("turn_id").MaxLen(64).Comment("turn that triggered the graph (plan creation turn)"),
		field.String("session_id").MaxLen(128).Comment("spirit_session_id"),
		field.String("plan_board_id").MaxLen(64).Unique().Comment("one-to-one with PlanBoardV2"),
		field.String("status").MaxLen(32).Default("running"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
	}
}

func (GraphStageV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "seq").StorageKey("idx_graph_stages_v2_task_seq"),
		index.Fields("session_id").StorageKey("idx_graph_stages_v2_session"),
	}
}
