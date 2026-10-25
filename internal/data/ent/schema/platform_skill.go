package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformSkill maps table skill (Ent type name avoids collisions).
type PlatformSkill struct {
	ent.Schema
}

func (PlatformSkill) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "skill"},
	}
}

func (PlatformSkill) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("skill_key").Unique().MaxLen(512),
		field.String("name").MaxLen(1024),
		field.Text("description").Default(""),
		field.String("status").Default("active"),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.Text("config_json").Default("{}"),
		field.Text("metadata_json").Default("{}"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
		field.String("parent_id").Default(""),
		field.String("level").Default(""),
		field.String("agent_id").Default(""),
		field.String("provider").Default(""),
		field.String("model").Default(""),
		field.String("kind").Default("markdown").MaxLen(64),
		field.String("risk_level").Default("low").MaxLen(32),
		field.String("entry_path").Default("SKILL.md").MaxLen(512),
		field.Bool("filesystem_missing").Default(false),
		field.String("visibility").Default("workspace").MaxLen(64),
		field.Text("fallback_config_json").StorageKey("default_config_json").Default("{}"),
		field.String("parent_version_id").Default("").MaxLen(256),
		field.Text("evolution_reason").Default(""),
		field.String("lifecycle_status").Default("active").MaxLen(64),
		// P2-B: tenant isolation. empty = shared/legacy (visible to all workspaces);
		// non-empty = tenant-private (visible only to owning workspace).
		field.String("workspace_id").Default("").Comment("owning workspace ID; empty = shared/system builtin"),
		// P0-7 批 D1（deferred ① 落账）：信任分级两列。可空无默认——NULL=未
		// 回填（存量行迁移前天然 NULL，value-guarded UPDATE 幂等回填 T1-T4/
		// biz 字面值，migration 20261297 + ddl 回填 hook 同 commit）。biz 侧
		// SkillTrust 结构不内嵌（§8.4② AS-COG-01：详情 DTO 透出另批）。
		field.Text("trust_level").Optional().Nillable().Comment("skill trust level T1-T4; NULL = not backfilled"),
		field.Text("source").Optional().Nillable().Comment("skill source builtin/llm_evolution/zip_import/user_created; NULL = not backfilled"),
	}
}

func (PlatformSkill) Indexes() []ent.Index {
	return []ent.Index{
		// P2-B: tenant isolation index — filter skills by workspace visibility.
		index.Fields("workspace_id", "enabled"),
	}
}
