package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MemberSessionV2 是 team 内的成员会话（v2 模型）。
type MemberSessionV2 struct {
	ent.Schema
}

func (MemberSessionV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "member_sessions_v2"},
	}
}

func (MemberSessionV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("team_run_id").MaxLen(64),
		field.String("team_stage_id").MaxLen(64),
		field.String("task_id").MaxLen(64),
		field.String("session_id").MaxLen(128).Comment("member own session for lazy load"),
		field.String("spirit_session_id").MaxLen(128),
		field.String("agent_key").MaxLen(128),
		field.String("agent_name").MaxLen(128).Default(""),
		field.String("avatar_url").MaxLen(512).Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
		field.Time("started_at").Default(timeNow).Comment("when member session started"),
		field.Time("finished_at").Optional().Nillable().Comment("when member session finished (null if not finished)"),
		field.String("error").Default("").Comment("error message if failed (empty if no error)"),
	}
}

func (MemberSessionV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_run_id", "seq").StorageKey("idx_member_sessions_v2_run_seq"),
		index.Fields("agent_key").StorageKey("idx_member_sessions_v2_agent"),
	}
}
