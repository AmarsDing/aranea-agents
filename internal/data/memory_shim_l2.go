package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// l2EpisodeRepo delegates L2 episode operations to sessionmemory.Store.
// Implements biz.L2EpisodeWriter + biz.L2ConsolidationStore + biz.L2RecallStore.
type l2EpisodeRepo struct {
	store *sessionmemory.Store
}

func newL2EpisodeRepo(store *sessionmemory.Store) *l2EpisodeRepo {
	if store == nil {
		return nil
	}
	return &l2EpisodeRepo{store: store}
}

// Compile-time interface checks.
var (
	_ biz.L2EpisodeWriter      = (*l2EpisodeRepo)(nil)
	_ biz.L2ConsolidationStore = (*l2EpisodeRepo)(nil)
	_ biz.L2RecallStore        = (*l2EpisodeRepo)(nil)
)

// L2EpisodeWriter

func (r *l2EpisodeRepo) InsertL1ArchiveEpisode(ctx context.Context, in biz.L1ArchiveEpisodeInsert) error {
	return r.store.InsertL1ArchiveEpisode(ctx, in)
}

// L2ConsolidationStore

func (r *l2EpisodeRepo) ListPendingConsolidationEpisodes(ctx context.Context, agentID string, limit int) ([][]byte, error) {
	return r.store.ListPendingConsolidationEpisodes(ctx, agentID, limit)
}

func (r *l2EpisodeRepo) MarkEpisodeConsolidated(ctx context.Context, id string, l3Count, l4Count int) error {
	return r.store.MarkEpisodeConsolidated(ctx, id, l3Count, l4Count)
}

// L2RecallStore

func (r *l2EpisodeRepo) ListEpisodeRowsForRecall(ctx context.Context, agentID, sessionID string, limit int32) ([][]byte, error) {
	return r.store.ListEpisodeRowsForRecall(ctx, agentID, sessionID, limit)
}

func (r *l2EpisodeRepo) RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	return r.store.RecallL2Episodes(ctx, agentID, sessionID, query, queryEmbedding, limit)
}
