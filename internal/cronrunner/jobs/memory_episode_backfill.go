package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const memoryEpisodeBackfillInterval = 6 * time.Hour

// MemoryEpisodeBackfillWorker embeds historical episodes missing embedding_blob.
type MemoryEpisodeBackfillWorker struct {
	interval    time.Duration
	reader      biz.MemoryEpisodeBackfillReader
	episodeSync biz.EpisodeIndexSyncer
	sys         biz.SystemSettingRepo
	lg          loggateway.Logger
}

func NewMemoryEpisodeBackfillWorker(interval time.Duration, reader biz.MemoryEpisodeBackfillReader, episodeSync biz.EpisodeIndexSyncer, sys biz.SystemSettingRepo, lg loggateway.Logger) *MemoryEpisodeBackfillWorker {
	if interval <= 0 {
		interval = memoryEpisodeBackfillInterval
	}
	return &MemoryEpisodeBackfillWorker{interval: interval, reader: reader, episodeSync: episodeSync, sys: sys, lg: lg}
}

func (w *MemoryEpisodeBackfillWorker) Start(ctx context.Context) {
	if w == nil || w.reader == nil || w.episodeSync == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *MemoryEpisodeBackfillWorker) runOnce(ctx context.Context) {
	if biz.ResolveEpisodeBackfillDisabled(ctx, w.sys) {
		return
	}
	safego.Go(ctx, "memory.episode_backfill", func() {
		cands, err := w.reader.ListEpisodesPendingEmbedding(ctx, 48)
		if err != nil {
			w.lg.Warn("list pending episodes failed", loggateway.Err(err))
			return
		}
		var n int64
		for _, c := range cands {
			if err := w.episodeSync.SyncEpisodeIndex(ctx, c.AgentID, c.ID, c.Title, c.Summary); err != nil {
				continue
			}
			n++
		}
		if n > 0 {
			biz.MemoryWorkerStatsGlobal().RecordEpisodeBackfill(n)
			w.lg.Info("memory episode backfill embedded episodes", loggateway.Int("count", int(n)))
		}
	})
}

func MemoryEpisodeBackfillDisabled() bool {
	return biz.ResolveEpisodeBackfillDisabled(context.Background(), nil)
}
