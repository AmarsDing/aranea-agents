package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// FederationPolicy maps the federation_policies table.
// Represents access control and quota policies between federated organizations.
type FederationPolicy struct {
	ent.Schema
}

func (FederationPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "federation_policies"},
	}
}

func (FederationPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64).DefaultFunc(uuid.NewString),
		field.String("caller_org_id").NotEmpty().MaxLen(64),                       // Source organization ID ("local" = this org)
		field.String("callee_org_id").NotEmpty().MaxLen(64),                       // Target organization ID
		field.Enum("action").Values("allow", "deny", "approval").Default("allow"), // approval reserved, treated as deny this iteration
		field.Int("max_per_min").Default(0),                                       // per-minute invocation cap (Limiter sliding-window semantics); 0 = unlimited
		field.Int("daily_quota").Default(0),                                       // daily invocation cap (count of decision=allowed); 0 = unlimited
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (FederationPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("caller_org_id", "callee_org_id").Unique().StorageKey("idx_federation_policies_pair"),
		index.Fields("caller_org_id").StorageKey("idx_federation_policies_caller"),
		index.Fields("callee_org_id").StorageKey("idx_federation_policies_callee"),
	}
}
