package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
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
func NewSessionAdminStoreAdapter(data *Data, vs vector.VectorStore) biz.SessionAdminStore {
	if data == nil {
		return nil
	}
	return &sessionAdminStoreAdapter{
		l0SnapshotRepo:      newL0SnapshotRepo(data),
		l1WorkingMemoryRepo: newL1WorkingMemoryRepo(data),
		l2EpisodeRepo:       newL2EpisodeRepo(data, vs),
		l3FactRepo:          newL3FactRepo(data, vs),
		l4EntityRepo:        newL4EntityRepo(data),
	}
}
