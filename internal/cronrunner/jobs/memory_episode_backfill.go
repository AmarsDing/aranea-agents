package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

const memoryEpisodeBackfillInterval = 6 * time.Hour

// MemoryEpisodeBackfillWorker embeds historical episodes missing embedding_blob.
type MemoryEpisodeBackfillWorker struct {
	interval    time.Duration
	reader      biz.MemoryEpisodeBackfillReader
	episodeSync biz.EpisodeIndexSyncer
	sys         biz.SystemSettingRepo
	log         *log.Helper
}

func NewMemoryEpisodeBackfillWorker(interval time.Duration, reader biz.MemoryEpisodeBackfillReader, episodeSync biz.EpisodeIndexSyncer, sys biz.SystemSettingRepo, logger log.Logger) *MemoryEpisodeBackfillWorker {
	if interval <= 0 {
		interval = memoryEpisodeBackfillInterval
	}
	return &MemoryEpisodeBackfillWorker{interval: interval, reader: reader, episodeSync: episodeSync, sys: sys, log: log.NewHelper(logger)}
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
			event.SysLogWarn("memory.episode_backfill", "list pending episodes failed", event.P("error", err))
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
			if w.log != nil {
				w.log.Infof("memory episode backfill: embedded %d episodes", n)
			}
		}
	})
}

func MemoryEpisodeBackfillDisabled() bool {
	return biz.ResolveEpisodeBackfillDisabled(context.Background(), nil)
}
