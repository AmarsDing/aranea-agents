package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TaskV2 是用户输入对应的根活动（v2 模型）。
type TaskV2 struct {
	ent.Schema
}

func (TaskV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tasks_v2"},
	}
}

func (TaskV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("session_id").MaxLen(128).Comment("spirit_session_id"),
		field.Text("user_message").Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Int64("seq").Default(0).Comment("Sequence within session, monotonic"),
		field.Int64("version").Default(0).Comment("Monotonic version for optimistic concurrency (VersionLT)"),
		// P2-B: tenant isolation. empty = legacy (treated as default workspace);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").MaxLen(128),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (TaskV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "seq").StorageKey("idx_tasks_v2_session_seq"),
		index.Fields("status").StorageKey("idx_tasks_v2_status"),
		// P2-B: workspace visibility filter.
		index.Fields("workspace_id", "status").StorageKey("idx_tasks_v2_workspace"),
	}
}
