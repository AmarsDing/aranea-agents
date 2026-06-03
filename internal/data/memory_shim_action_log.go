package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// actionLogRepo delegates policy audit writes to sessionmemory.Store.
// Implements biz.MemoryActionLogWriter.
type actionLogRepo struct {
	store *sessionmemory.Store
}

func newActionLogRepo(store *sessionmemory.Store) *actionLogRepo {
	if store == nil {
		return nil
	}
	return &actionLogRepo{store: store}
}

// NewMemoryActionLogWriter creates a biz.MemoryActionLogWriter backed by sessionmemory.Store.
// Returns nil if store is nil.
func NewMemoryActionLogWriter(store *sessionmemory.Store) biz.MemoryActionLogWriter {
	if store == nil {
		return nil
	}
	return newActionLogRepo(store)
}

// Compile-time interface check.
var _ biz.MemoryActionLogWriter = (*actionLogRepo)(nil)

func (r *actionLogRepo) WriteMemoryActionLog(ctx context.Context, rec biz.MemoryPolicyRecord) error {
	return r.store.WriteMemoryActionLog(ctx, rec)
}
