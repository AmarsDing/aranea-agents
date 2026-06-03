package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// sessionAdminStoreAdapter composes all memory shim repos to implement biz.SessionAdminStore.
type sessionAdminStoreAdapter struct {
	*l0SnapshotRepo
	*l1WorkingMemoryRepo
	*l2EpisodeRepo
	*l3FactRepo
	*l4EntityRepo
}

// Compile-time interface check.
var _ biz.SessionAdminStore = (*sessionAdminStoreAdapter)(nil)

// NewSessionAdminStoreAdapter creates a SessionAdminStore by composing all shim repos.
// Returns nil if store is nil.
func NewSessionAdminStoreAdapter(store *sessionmemory.Store) biz.SessionAdminStore {
	if store == nil {
		return nil
	}
	return &sessionAdminStoreAdapter{
		l0SnapshotRepo:      newL0SnapshotRepo(store),
		l1WorkingMemoryRepo: newL1WorkingMemoryRepo(store),
		l2EpisodeRepo:       newL2EpisodeRepo(store),
		l3FactRepo:          newL3FactRepo(store),
		l4EntityRepo:        newL4EntityRepo(store),
	}
}
