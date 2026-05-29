package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

type memoryEpisodeIndexSync struct {
	vec   *biz.MemoryUsecase
	store *sessionmemory.Store
}

var _ biz.EpisodeIndexSyncer = (*memoryEpisodeIndexSync)(nil)

func NewMemoryEpisodeIndexSync(vec *biz.MemoryUsecase, store *sessionmemory.Store) biz.EpisodeIndexSyncer {
	if vec == nil || store == nil {
		return nil
	}
	return &memoryEpisodeIndexSync{vec: vec, store: store}
}

func (s *memoryEpisodeIndexSync) SyncEpisodeIndex(ctx context.Context, _ string, episodeID, title, summary string) error {
	if s == nil || s.vec == nil || s.store == nil {
		return biz.ErrMemoryUnavailable
	}
	episodeID = strings.TrimSpace(episodeID)
	text := strings.TrimSpace(strings.TrimSpace(title) + "\n" + strings.TrimSpace(summary))
	if episodeID == "" || text == "" {
		return nil
	}
	embedder := s.vec
	return syncEpisodeEmbedding(ctx, embedder, s.store, episodeID, text)
}

func syncEpisodeEmbedding(ctx context.Context, embedder biz.EmbeddingService, store *sessionmemory.Store, episodeID, text string) error {
	if embedder == nil {
		return biz.ErrMemoryUnavailable
	}
	embedding, err := embedder.Embed(ctx, text)
	if err != nil {
		return err
	}
	return store.UpsertEpisodeEmbedding(ctx, episodeID, embedding, "memory_embedder", len(embedding))
}
