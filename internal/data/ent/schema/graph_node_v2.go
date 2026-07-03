package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GraphNodeV2 是 GraphStage 内的一个节点，对应一个 PlanStep。
// 节点状态由 PlanStep.Status 通过 MapPlanStepToGraphNodeStatus 映射得到，
// 由 PlanExecutor 在 dispatchStep/checkDownstream 时同步更新。
//
// 设计：见 docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
// §3.2.3 graph_nodes 表 / §3.7.5 GraphStageBlock 与 PlanDAG 的关系
//
// GraphNode.DependsOn 是 in-memory 字段（从 plan_steps.depends_on 派生），不在此表持久化。
type GraphNodeV2 struct {
	ent.Schema
}

func (GraphNodeV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "graph_nodes_v2"},
	}
}

func (GraphNodeV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("graph_stage_id").MaxLen(64),
		field.String("dag_node_id").MaxLen(64).Comment("corresponding plan_step.id"),
		field.String("team_stage_id").MaxLen(64).Default("").Comment("back-filled when team_stage created"),
		field.String("label").MaxLen(256).Default(""),
		field.String("status").MaxLen(32).Default("pending"),
	}
}

func (GraphNodeV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("graph_stage_id").StorageKey("idx_graph_nodes_v2_stage"),
		index.Fields("dag_node_id").StorageKey("idx_graph_nodes_v2_dag_node"),
	}
}
