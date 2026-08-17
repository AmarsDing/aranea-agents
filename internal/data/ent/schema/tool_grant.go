package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ToolGrant is a persisted "always allow" grant recorded when a user
// approves a tool confirmation with the always scope. Presence of a row
// skips the confirmation prompt for the (agent_id, tool_key) pair.
type ToolGrant struct {
	ent.Schema
}

func (ToolGrant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tool_grants"},
	}
}

func (ToolGrant) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("agent_id"),
		field.String("tool_key"),
		// granted_by records who approved the grant (user id when known,
		// otherwise the session id) for audit.
		field.String("granted_by").Default(""),
		field.String("created_at"),
		// expires_at is the RFC3339 UTC deadline of the grant; '' means
		// never expires (reserved for a future explicit "permanent" option).
		// Grants written via ToolUsecase.GrantTool get now+72h
		// (BUG-MON-B, 2026-08-17): persisted "always allow" must have a
		// determined time bound, read paths filter expired rows.
		field.String("expires_at").Default(""),
	}
}

func (ToolGrant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_id", "tool_key").Unique(),
		index.Fields("agent_id"),
	}
}
