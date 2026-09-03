package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamRunV2 是 team 内的一次执行（v2 模型）。
type TeamRunV2 struct {
	ent.Schema
}

func (TeamRunV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_runs_v2"},
	}
}

func (TeamRunV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("team_stage_id").MaxLen(64),
		field.String("task_id").MaxLen(64),
		field.String("session_id").MaxLen(128),
		field.String("spirit_session_id").MaxLen(128),
		field.String("dag_node_id").MaxLen(128).Default(""),
		field.JSON("depends_on", []string{}).Optional(),
		field.String("status").MaxLen(32).Default("running"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
		field.String("error").Default("").Comment("error message if failed (empty if no error)"),
		// P2-1（2026-09-03 持久化心跳）：runner 流式消费节流写入；idle 探测
		// （spirit_orchestration.probeTeamActivity）优先取 max(steps.started_at,
		// heartbeat_at)，根治「单 step 长流式生成被误判 idle」。
		field.Time("heartbeat_at").Optional().Nillable().Comment("runner streaming heartbeat (throttled write)"),
	}
}

func (TeamRunV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_stage_id", "seq").StorageKey("idx_team_runs_v2_stage_seq"),
		index.Fields("dag_node_id").StorageKey("idx_team_runs_v2_dag_node"),
	}
}
