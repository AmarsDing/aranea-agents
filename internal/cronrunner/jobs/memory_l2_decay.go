package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	memoryL2DecayDefaultInterval = 24 * time.Hour
	memoryL2DecayMinEpisodeAge   = 24 * time.Hour
	memoryL2DecayBatchFactor     = 0.95
)

// MemoryL2DecayWorker periodically reduces stored episode importance and purges episodes past retention.
type MemoryL2DecayWorker struct {
	interval time.Duration
	decayer  biz.MemoryEpisodeDecayer
	agents   *biz.AgentUsecase
	lg       loggateway.Logger
}

func NewMemoryL2DecayWorker(interval time.Duration, decayer biz.MemoryEpisodeDecayer, agents *biz.AgentUsecase, lg loggateway.Logger) *MemoryL2DecayWorker {
	if interval <= 0 {
		interval = memoryL2DecayDefaultInterval
	}
	return &MemoryL2DecayWorker{
		interval: interval,
		decayer:  decayer,
		agents:   agents,
		lg:       lg,
	}
}

func (w *MemoryL2DecayWorker) Start(ctx context.Context) {
	if w == nil || w.decayer == nil {
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

func (w *MemoryL2DecayWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.l2_decay", func() {
		if w.agents != nil {
			targets, err := w.agents.ListMemoryMaintenanceTargets(ctx)
			if err != nil {
				w.lg.Warn("L2 maintenance target list failed", loggateway.Err(err))
				return
			}
			decayCutoff := time.Now().UTC().Add(-memoryL2DecayMinEpisodeAge).Format(time.RFC3339Nano)
			var decayed, purged int
			for _, t := range targets {
				if !t.WriteL2Episode {
					continue
				}
				if n, err := w.decayer.ApplyEpisodeImportanceDecay(ctx, t.AgentID, decayCutoff, memoryL2DecayBatchFactor); err != nil {
					w.lg.Warn("L2 episode importance decay failed", loggateway.Str("agent_id", t.AgentID), loggateway.Err(err))
				} else {
					decayed += n
				}
				retentionDays := t.L2RetentionDays
				if retentionDays <= 0 {
					retentionDays = 90
				}
				purgeCutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).Format(time.RFC3339Nano)
				if n, err := w.decayer.PurgeEpisodesOlderThan(ctx, t.AgentID, purgeCutoff); err != nil {
					w.lg.Warn("L2 episode retention purge failed", loggateway.Str("agent_id", t.AgentID), loggateway.Err(err))
				} else {
					purged += n
				}
			}
			if decayed > 0 || purged > 0 {
				w.lg.Info("memory l2 decay completed", loggateway.Int("importance", decayed), loggateway.Int("purged", purged), loggateway.Int("agents", len(targets)))
			}
			return
		}
		cutoff := time.Now().UTC().Add(-memoryL2DecayMinEpisodeAge).Format(time.RFC3339Nano)
		n, err := w.decayer.ApplyAllEpisodeImportanceDecay(ctx, cutoff, memoryL2DecayBatchFactor)
		if err != nil {
			w.lg.Warn("L2 episode importance decay failed", loggateway.Err(err))
			return
		}
		if n > 0 {
			w.lg.Info("memory l2 decay updated episodes", loggateway.Int("count", n), loggateway.Str("cutoff", cutoff), loggateway.Any("factor", memoryL2DecayBatchFactor))
		}
	})
}

func MemoryL2DecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L2_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
