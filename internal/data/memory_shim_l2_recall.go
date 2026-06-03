package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// l2RecallRepo delegates L2 recall operations to sessionmemory.Store.
// Implements biz.SessionL2RecallStore.
type l2RecallRepo struct {
	store *sessionmemory.Store
}

func newL2RecallRepo(store *sessionmemory.Store) *l2RecallRepo {
	if store == nil {
		return nil
	}
	return &l2RecallRepo{store: store}
}

// NewSessionL2RecallStore creates a biz.SessionL2RecallStore backed by sessionmemory.Store.
// Returns nil if store is nil.
func NewSessionL2RecallStore(store *sessionmemory.Store) biz.SessionL2RecallStore {
	if store == nil {
		return nil
	}
	return newL2RecallRepo(store)
}

// Compile-time interface check.
var _ biz.SessionL2RecallStore = (*l2RecallRepo)(nil)

func (r *l2RecallRepo) RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	return r.store.RecallL2Episodes(ctx, agentID, sessionID, query, queryEmbedding, limit)
}
