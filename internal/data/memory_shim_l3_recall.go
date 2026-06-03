package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// l3RecallRepo delegates L3 recall operations to sessionmemory.Store.
// Implements biz.SessionL3RecallStore.
type l3RecallRepo struct {
	store *sessionmemory.Store
}

func newL3RecallRepo(store *sessionmemory.Store) *l3RecallRepo {
	if store == nil {
		return nil
	}
	return &l3RecallRepo{store: store}
}

// NewSessionL3RecallStore creates a biz.SessionL3RecallStore backed by sessionmemory.Store.
// Returns nil if store is nil.
func NewSessionL3RecallStore(store *sessionmemory.Store) biz.SessionL3RecallStore {
	if store == nil {
		return nil
	}
	return newL3RecallRepo(store)
}

// Compile-time interface check.
var _ biz.SessionL3RecallStore = (*l3RecallRepo)(nil)

func (r *l3RecallRepo) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	return r.store.RecallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
}
