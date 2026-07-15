package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type TaskPlan struct {
	ent.Schema
}

func (TaskPlan) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "task_plans"}}
}

func (TaskPlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(256).Immutable().Unique(),
		field.String("spirit_session_id").MaxLen(256).Default(""),
		field.String("trace_id").MaxLen(128).Default(""),
		field.Text("user_message").Default(""),
		field.Text("intent_artifact_json").Default("{}"),
		field.String("complexity_level").MaxLen(32).Default("simple"),
		field.Float("complexity_score").Default(0),
		field.Text("dimensions_json").Default("{}"),
		field.Text("sub_tasks_json").Default("[]"),
		field.Text("dag_json").Default("{}"),
		field.Text("decompose_reason").Default(""),
		field.String("strategy").MaxLen(64).Default("direct"),
		field.Text("strategy_reason").Default(""),
		field.String("topology_hint").MaxLen(64).Default(""),
		field.Text("memory_hit_json").Default("{}"),
		field.String("status").MaxLen(32).Default("draft"),
		// P2-B: tenant isolation. empty = legacy (treated as default workspace);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").MaxLen(128),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}

func (TaskPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("spirit_session_id"),
		// P2-B: workspace visibility filter.
		index.Fields("workspace_id", "status").StorageKey("idx_task_plans_workspace"),
	}
}
