package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// FederationOrg maps the federation_orgs table.
// Represents a federated organization in the A2A federation network.
type FederationOrg struct {
	ent.Schema
}

func (FederationOrg) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "federation_orgs"},
	}
}

func (FederationOrg) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(64).DefaultFunc(uuid.NewString),
		field.String("name").NotEmpty().MaxLen(256),
		field.String("domain").NotEmpty().MaxLen(512), // Unique domain identifier
		field.String("public_base_url").Default("").MaxLen(1024),
		field.Enum("trust_level").Values("untrusted", "neutral", "trusted").Default("neutral"),
		field.String("auth_type").Default("").MaxLen(32), // none | api_key | bearer | mtls
		field.Text("auth_config_json").Default("{}").Sensitive(), // DB-N8: sensitive field
		field.Enum("status").Values("active", "suspended").Default("active"),
		field.Time("joined_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (FederationOrg) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("domain").Unique().StorageKey("idx_federation_orgs_domain"),
		index.Fields("status").StorageKey("idx_federation_orgs_status"),
		index.Fields("trust_level").StorageKey("idx_federation_orgs_trust_level"),
	}
}
