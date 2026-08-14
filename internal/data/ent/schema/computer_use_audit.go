package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ComputerUseAudit maps table computer_use_audit（75-computer-use 审计落库）。
type ComputerUseAudit struct {
	ent.Schema
}

func (ComputerUseAudit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "computer_use_audit"},
	}
}

func (ComputerUseAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("agent_key"),
	}
}

func (ComputerUseAudit) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id").MaxLen(64),
		field.String("agent_key").MaxLen(256),
		field.Int("step_index"),
		field.Text("target").Default(""),
		field.String("path").MaxLen(16).Default("a11y"), // a11y|vision|vlm_direct|grounder
		field.String("action").MaxLen(32),
		field.JSON("params", map[string]any{}).Optional(),
		field.String("result").MaxLen(16).Default(""), // ok|retry|failed|cancelled|dry_run
		field.Text("error").Default(""),
		field.Int64("duration_ms").Default(0),
		field.String("confirmed_by").MaxLen(256).Default(""),
		field.Bool("danger").Default(false),
		field.String("screenshot_ref").MaxLen(512).Default(""), // 审计截图文件路径（AuditShotDir）
		field.Time("created_at"),
	}
}
