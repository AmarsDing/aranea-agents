package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TurnV2 是最小对话单元（v2 模型）。
type TurnV2 struct {
	ent.Schema
}

func (TurnV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "turns_v2"},
	}
}

func (TurnV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("session_id").MaxLen(128).Comment("current session (spirit/team/member)"),
		field.String("spirit_session_id").MaxLen(128).Comment("always points to spirit root session for WS filter"),
		field.String("parent_turn_id").MaxLen(64).Default("").Comment("nested turn parent"),
		field.String("agent_key").MaxLen(128).Default(""),
		field.String("team_id").MaxLen(128).Default("").Comment("empty for spirit turn"),
		field.String("team_stage_id").MaxLen(64).Default("").Comment("set when member turn"),
		field.Int64("seq").Default(0).Comment("global seq within task, monotonic"),
		field.String("status").MaxLen(32).Default("running"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (TurnV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "seq").StorageKey("idx_turns_v2_task_seq"),
		index.Fields("spirit_session_id", "seq").StorageKey("idx_turns_v2_spirit_seq"),
		index.Fields("parent_turn_id").StorageKey("idx_turns_v2_parent"),
	}
}
