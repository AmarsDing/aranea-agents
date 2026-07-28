package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// FederationAuditLog maps the federation_audit_logs table.
// Audit records for cross-organization A2A invocations (including denied calls).
type FederationAuditLog struct {
	ent.Schema
}

func (FederationAuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "federation_audit_logs"},
	}
}

func (FederationAuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64).DefaultFunc(uuid.NewString),
		field.Enum("direction").Values("outbound", "inbound").Default("outbound"), // inbound reserved for future iteration
		field.String("caller_org_id").NotEmpty().MaxLen(64),
		field.String("callee_org_id").NotEmpty().MaxLen(64),
		field.String("caller_agent_id").Default("").MaxLen(64),
		field.String("callee_agent_id").Default("").MaxLen(64),
		field.String("capability").NotEmpty().MaxLen(256),
		field.Enum("decision").Values("allowed", "denied_trust", "denied_policy", "denied_quota"),
		field.Enum("status").Values("pending", "success", "error", "timeout").Default("pending"),
		field.Int64("latency_ms").Default(0),
		field.Text("error_message").Default(""),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (FederationAuditLog) Indexes() []ent.Index {
	return []ent.Index{
		// Audit queries + daily-quota counting scan by org pair ordered by created_at.
		index.Fields("caller_org_id", "created_at").StorageKey("idx_federation_audit_logs_caller_created"),
		index.Fields("callee_org_id", "created_at").StorageKey("idx_federation_audit_logs_callee_created"),
	}
}
