package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Agent maps legacy table agents (read paths + counts only until full agent domain is migrated).
type Agent struct {
	ent.Schema
}

func (Agent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agents"},
	}
}

func (Agent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("deleted_at"),
		index.Fields("deleted_at", "status"),
		// BUG-01 (2026-08-17): partial unique — 无岗位 agent（position_key=''）与软删行
		// 不占用岗位槽位唯一键；设计意图仅约束「一岗一变体一在任」。
		index.Fields("position_key", "agent_variant").Unique().
			Annotations(entsql.IndexWhere("position_key <> '' AND deleted_at = ''")),
		// P2-B: tenant isolation index — filter agents by workspace visibility.
		index.Fields("workspace_id", "deleted_at"),
	}
}

func (Agent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("agent_key").Unique().MaxLen(512),
		field.String("display_name").MaxLen(1024),
		field.String("provider"),
		field.String("model"),
		field.String("status").Default("active"),
		field.Bool("is_default").Default(false),
		field.Bool("is_favorite").Default(false),
		field.String("icon").Default(""),
		field.Text("agent_description").Default(""),
		field.String("system_prompt_mode").Default(""),
		field.Int("context_window").Default(0),
		field.Int("budget_monthly_cents").Default(0),
		field.Text("config_json").Default(""),
		field.Text("roles_json").Default("[]"),
		field.String("created_by").Default("").Comment("creator user id from auth context"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
		// CLI-20: system admin agent support.
		field.Bool("readonly").Default(false).Comment("system agents cannot be deleted"),
		field.Enum("kind").Values("user", "system_builtin", "ecosystem_preset", "marketplace", "certified").Default("user").Comment("agent kind: ownership classification (user | system_builtin | ecosystem_preset | marketplace | certified)"),
		field.Enum("source").Values("user", "system", "imported").Default("user").Comment("agent source: origin tracking (user | system | imported), aligned with team.source"),
		field.String("position_key").Default("").Comment("FK to positions.key"),
		field.String("position_id").Default("").Comment("FK to organizations(position), renamed from taxonomy_position_id"),
		field.String("agent_variant").Default("general").Comment("variant within position: general/code_review/architect/..."),
		field.Text("variant_description").Default("").Comment("human-readable description of this variant"),
		// P2-B: tenant isolation. empty = shared/legacy (visible to all workspaces);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").Comment("owning workspace ID; empty = shared/system builtin"),
		// B.10.21: mission-driven matching. 出生登记于 AgentFactory；手工 Agent 默认空。
		field.Text("mission_statement").Default("").Comment("agent 长期使命陈述（使命驱动匹配）"),
		field.String("domain_path").Default("").MaxLen(256).Comment("归一化领域路径（如 创作/文学）"),
	}
}
