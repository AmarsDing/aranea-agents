package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

type memoryEpisodeIndexSync struct {
	vec  *biz.MemoryUsecase
	data *Data
}

var _ biz.EpisodeIndexSyncer = (*memoryEpisodeIndexSync)(nil)

func NewMemoryEpisodeIndexSync(vec *biz.MemoryUsecase, data *Data) biz.EpisodeIndexSyncer {
	if vec == nil || data == nil {
		return nil
	}
	return &memoryEpisodeIndexSync{vec: vec, data: data}
}

func (s *memoryEpisodeIndexSync) SyncEpisodeIndex(ctx context.Context, _ string, episodeID, title, summary string) error {
	if s == nil || s.vec == nil || s.data == nil {
		return biz.ErrMemoryUnavailable
	}
	episodeID = strings.TrimSpace(episodeID)
	text := strings.TrimSpace(strings.TrimSpace(title) + "\n" + strings.TrimSpace(summary))
	if episodeID == "" || text == "" {
		return nil
	}
	embedding, err := s.vec.Embed(ctx, text)
	if err != nil {
		return err
	}
	blob := encodeFloat32Blob(embedding)
	norm := vectorL2Norm(embedding)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		s.data.Dialect().RenumberPlaceholders(`UPDATE memory_episodes SET embedding_blob = ?, embedding_norm = ?, embedding_dim = ?, embedding_status = 'fresh', embedding_model = 'memory_embedder', updated_at = ? WHERE id = ?`),
		blob, norm, len(embedding), now, episodeID)
	return err
}
