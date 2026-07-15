package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Team struct {
	ent.Schema
}

func (Team) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "teams"},
	}
}

func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("team_key").Unique().MaxLen(512),
		field.String("display_name").MaxLen(1024),
		field.String("status").Default("pending"),
		field.Bool("is_default").Default(false),
		field.Text("definition_json").Default(""),
		field.String("adk_app_name").Default(""),
		field.String("department_id").Default("").Comment("FK to organizations(department), renamed from category_industry_id"),
		field.String("spirit_session_id").Default("").MaxLen(256),
		field.Text("task_description").Default(""),
		field.Bool("auto_created").Default(false),
		field.String("dag_node_id").Default("").MaxLen(256),
		field.Text("depends_on_json").Default(""),
		field.Text("parallel_config_json").Default(""),
		field.String("topology").Default("").MaxLen(64),
		field.Bool("readonly").Default(false).Comment("system teams cannot be deleted"),
		field.Enum("kind").Values("user", "system_builtin", "ecosystem_preset", "marketplace", "certified").Default("user").Comment("team kind: aligned with agent.kind for unified permission model"),
		field.Enum("source").Values("user", "system", "imported").Default("user").Comment("team source: user | system | imported"),
		// organization redesign: deliverable contract + dept lead + cross-dept members
		field.Text("deliverables").Default("[]").Comment("deliverable definitions JSON"),
		field.Text("input_contract").Default("[]").Comment("input contract JSON (expected from upstream teams)"),
		field.String("dept_lead_agent_id").Default("").Optional().Comment("department lead Agent ID for this team (inherited from department by default)"),
		field.Text("cross_dept_member_ids").Default("[]").Comment("cross-department member Agent ID list JSON"),
		field.String("linked_graph_id").Default("").Optional().Comment("FK to graph_definitions(id); bidirectional reference with graph.team_id"),
		field.String("interrupt_reason").Default("").MaxLen(1024).Optional().Comment("reason for team interruption (e.g. server restart, user cancel)"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
		// P2-B: tenant isolation. empty = shared/legacy (visible to all workspaces);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").Comment("owning workspace ID; empty = shared/system builtin"),
	}
}

func (Team) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("spirit_session_id", "deleted_at").StorageKey("idx_teams_spirit_session"),
		index.Fields("kind", "deleted_at").StorageKey("idx_teams_kind"),
		// P2-B: tenant isolation index — filter teams by workspace visibility.
		index.Fields("workspace_id", "deleted_at"),
	}
}
