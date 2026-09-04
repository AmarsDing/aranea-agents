package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// StepV2 是 turn 内的工作步骤（v2 模型）。
type StepV2 struct {
	ent.Schema
}

func (StepV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "steps_v2"},
	}
}

func (StepV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("turn_id").MaxLen(64),
		field.String("task_id").MaxLen(64).Comment("redundant for task indexing"),
		field.String("session_id").MaxLen(128),
		field.String("spirit_session_id").MaxLen(128),
		field.String("kind").MaxLen(32).Comment("thinking/action/reply/notice/confirm/error"),
		field.String("author_agent_key").MaxLen(128).Default("").Comment("agent key that authored this step"),
		field.Int64("seq").Default(0).Comment("seq within turn"),
		field.Text("content").Default(""),
		field.Text("reasoning").Default(""),
		field.String("tool_name").MaxLen(128).Default(""),
		field.String("tool_call_id").MaxLen(512).Default(""),
		field.Text("tool_args").Default("").Sensitive(),
		field.Text("tool_result").Default("").Sensitive(),
		field.Int64("tool_duration_ms").Default(0),
		field.String("tool_error_code").MaxLen(64).Default(""),
		field.String("notice_type").MaxLen(64).Default("").Comment("kind=notice: notification type (e.g. model_router, cost_guard)"),
		field.String("status").MaxLen(32).Default("pending"),
		field.Bool("is_final").Default(false).Comment("reply: is this the final reply"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		// P4-1（2026-09-03 观测面）：updated_at 由 ent 在每次写入时自动刷新
		// （UpdateDefault），无需业务调用点改动。观测/审计可见行级新鲜度；
		// idle 探测取 max(started_at, updated_at)（step_v2_repo.
		// LatestStepActivityAt），覆盖「step 原地更新但无新 step」的活跃形态。
		// entsql.Default("now()") 必须显式给：.Default(timeNow) 只是 Go 侧默认，
		// 不落 DB DDL——ent 启动迁移对存量表 ADD COLUMN NOT NULL 无 DEFAULT 会
		// 直接 23502 崩溃（2026-09-03 108 实证）。
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow).
			Annotations(entsql.Default("now()")),
		field.Int64("version").Default(0).Comment("Monotonic version for ordered upserts"),
	}
}

func (StepV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("turn_id", "seq").StorageKey("idx_steps_v2_turn_seq"),
		index.Fields("task_id", "seq").StorageKey("idx_steps_v2_task_seq"),
		index.Fields("spirit_session_id", "seq").StorageKey("idx_steps_v2_spirit_seq"),
	}
}
