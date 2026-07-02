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
		field.String("status").MaxLen(32).Default("pending"),
		field.Bool("is_final").Default(false).Comment("reply: is this the final reply"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
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
