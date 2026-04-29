package app

import (
	"arenea/backend/internal/capability"
	"arenea/backend/internal/catalog"
	"arenea/backend/internal/conversation"
	"arenea/backend/internal/identity"
	"arenea/backend/internal/kernel/module"
	"arenea/backend/internal/memory"
	"arenea/backend/internal/operations"
)

// InitModules instantiates one Module per bounded Context, in canonical
// composition order. The order MUST match the dependency direction:
// foundational ports first (identity), then producers (catalog, capability,
// memory), then consumers (conversation), then cross-cutting (operations).
//
// Skeleton state (P0): each NewModule constructor takes no dependencies yet;
// they will accept a *Container as ports begin to migrate.
func InitModules(c *Container) []module.Module {
	return []module.Module{
		identity.NewModule(),
		catalog.NewModule(),
		capability.NewModule(),
		memory.NewModule(),
		conversation.NewModule(),
		operations.NewModule(),
	}
}
