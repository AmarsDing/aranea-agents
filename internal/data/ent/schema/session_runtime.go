package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionRuntime maps table session_runtime — 运行时快照字段拆分表.
type SessionRuntime struct {
	ent.Schema
}

func (SessionRuntime) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "session_runtime"},
	}
}

func (SessionRuntime) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("id").StorageKey("idx_session_runtime_session"),
	}
}

func (SessionRuntime) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("session_id").Unique().Immutable().MaxLen(256),
		field.Int("session_revision").Default(0),
		field.Text("state_json").Default("{}"),
		field.Text("runner_snapshot_json").Default(""),
		field.Text("metadata_json").Default("{}"),
		field.Int("compress_version").Default(0),
		field.String("updated_at").Default(""),
	}
}
