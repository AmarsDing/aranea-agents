package memory

import (
	"aranea-agents/internal/data/sessionmemory"
	memtrpc "aranea-agents/internal/memory/trpc"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// RuntimeSet groups trpc runner memory (tool-facing) and session admin storage (L0–L4 APIs).
type RuntimeSet struct {
	TRPC  trpcmemory.Service
	Admin SessionAdminStore
}

// NewRuntimeSet wires both ports from the data-layer session memory store.
func NewRuntimeSet(store *sessionmemory.Store) RuntimeSet {
	if store == nil {
		return RuntimeSet{}
	}
	return RuntimeSet{
		TRPC:  memtrpc.NewSQLiteMemoryService(store),
		Admin: WrapSessionAdminStore(store),
	}
}

// Available reports whether session memory is configured for this process.
func (s RuntimeSet) Available() bool {
	return s.TRPC != nil
}
