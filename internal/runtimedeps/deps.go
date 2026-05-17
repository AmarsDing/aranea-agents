// Deprecated: use aranea-agents/internal/runtime instead.
// This package is a thin alias layer kept for one Sprint to avoid a flag-day migration.
package runtimedeps

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
	rt "aranea-agents/internal/runtime"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// Deprecated: use runtime.PersistenceSet.
type Runtime = rt.PersistenceSet

// Deprecated: use runtime.TurnDeps.
type TurnDeps = rt.TurnDeps

// NewRuntime constructs a PersistenceSet (formerly Runtime).
// Deprecated: use runtime.NewPersistenceSet.
func NewRuntime(store *sessionmemory.Store, mcp *biz.AgentMCPTooling, trpcSession trpcsession.Service) *Runtime {
	p := rt.NewPersistenceSet(store, mcp, trpcSession)
	return &p
}
