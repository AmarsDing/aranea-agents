package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamStageV2 是 task 内的团队执行阶段（v2 模型）。
type TeamStageV2 struct {
	ent.Schema
}

func (TeamStageV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_stages_v2"},
	}
}

func (TeamStageV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("turn_id").MaxLen(64).Comment("turn that triggered the team"),
		field.String("session_id").MaxLen(128).Comment("spirit_session_id"),
		field.String("team_id").MaxLen(128),
		field.String("team_name").MaxLen(256).Default("").Comment("team display name for UI (2026-07-04 问题 3)"),
		field.String("dag_node_id").MaxLen(128).Default("").Comment("corresponding plan_step.id"),
		field.JSON("depends_on", []string{}).Optional().Comment("other team_stage.id DAG deps"),
		field.String("status").MaxLen(32).Default("pending"),
		field.String("stage").MaxLen(64).Default("").Comment("assembled/planning/executing/completed/failed"),
		field.JSON("members", []map[string]any{}).Optional().Comment("type-safe member list"),
		field.String("strategy").MaxLen(32).Default("").Comment("parallel/dag/coordinator"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
	}
}

func (TeamStageV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "seq").StorageKey("idx_team_stages_v2_task_seq"),
		// Note: session_id field holds spirit_session_id value (see field comment).
		// Index uses session_id to match the actual field name.
		index.Fields("session_id", "seq").StorageKey("idx_team_stages_v2_spirit_seq"),
		index.Fields("dag_node_id").StorageKey("idx_team_stages_v2_dag_node"),
	}
}
