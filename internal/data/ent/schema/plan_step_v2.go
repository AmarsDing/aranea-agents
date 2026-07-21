package schema

import (
	"aranea-agents/internal/biz"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlanStepV2 是计划步骤（v2 模型）。
type PlanStepV2 struct {
	ent.Schema
}

func (PlanStepV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "plan_steps_v2"},
	}
}

func (PlanStepV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("plan_id").MaxLen(64),
		field.String("task_id").MaxLen(64).Comment("redundant for task indexing"),
		field.String("label").MaxLen(256).Default(""),
		field.Text("description").Default(""),
		field.JSON("depends_on", []string{}).Optional(),
		field.String("mapped_team_stage_id").MaxLen(64).Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Bool("auto_synthesis").Default(false).Comment("synthesis step, no team mapping"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
		field.JSON("result", map[string]any{}).Optional().Comment("StepResult JSON"),
		field.JSON("error", map[string]any{}).Optional().Comment("StepError JSON"),
		field.JSON("agent_keys", []string{}).Optional().Comment("LLM-allocated agent keys from AllocationPlan"),
		// P1 形式契约（B.10.15.2）：deliverables/input_contract 与 biz.PlanStep
		// 契约字段对应，crash recovery 重建 dagRun 时需要。
		field.JSON("deliverables", []biz.DeliverableContract{}).Optional().Comment("P1 output contract (DeliverableContract JSON array)"),
		field.JSON("input_contract", []biz.DeliverableContract{}).Optional().Comment("P1 input contract (DeliverableContract JSON array)"),
	}
}

func (PlanStepV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("plan_id", "seq").StorageKey("idx_plan_steps_v2_plan_seq"),
		index.Fields("task_id").StorageKey("idx_plan_steps_v2_task"),
		index.Fields("mapped_team_stage_id").StorageKey("idx_plan_steps_v2_team_stage"),
	}
}
