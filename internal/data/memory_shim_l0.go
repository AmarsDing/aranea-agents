package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// l0SnapshotRepo delegates L0 snapshot operations to sessionmemory.Store.
// Implements biz.L0AdminStore.
type l0SnapshotRepo struct {
	store *sessionmemory.Store
}

func newL0SnapshotRepo(store *sessionmemory.Store) *l0SnapshotRepo {
	if store == nil {
		return nil
	}
	return &l0SnapshotRepo{store: store}
}

// Compile-time interface check.
var _ biz.L0AdminStore = (*l0SnapshotRepo)(nil)

func (r *l0SnapshotRepo) ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error) {
	return r.store.ListL0SnapshotRows(ctx, sessionID, limit)
}

func (r *l0SnapshotRepo) GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	return r.store.GetL0SnapshotRow(ctx, sessionID, id)
}

func (r *l0SnapshotRepo) InsertL0AssemblySnapshot(ctx context.Context, in biz.L0AssemblySnapshotInsert) error {
	return r.store.InsertL0AssemblySnapshot(ctx, in)
}

func (r *l0SnapshotRepo) UpdateL0SnapshotActual(ctx context.Context, id string, actualPromptTokens, contextWindowTokens int) error {
	return r.store.UpdateL0SnapshotActual(ctx, id, actualPromptTokens, contextWindowTokens)
}
