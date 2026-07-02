package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlanBoardV2 是任务计划面板（v2 模型）。
type PlanBoardV2 struct {
	ent.Schema
}

func (PlanBoardV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "plan_boards_v2"},
	}
}

func (PlanBoardV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("turn_id").MaxLen(64).Comment("turn that triggered the plan"),
		field.String("session_id").MaxLen(128).Comment("spirit_session_id"),
		field.String("strategy").MaxLen(32).Default("sequential"),
		field.String("status").MaxLen(32).Default("planning"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
	}
}

func (PlanBoardV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "seq").StorageKey("idx_plan_boards_v2_task_seq"),
	}
}
