package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
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
	log      *log.Helper
}

func NewMemoryL2DecayWorker(interval time.Duration, decayer biz.MemoryEpisodeDecayer, agents *biz.AgentUsecase, logger log.Logger) *MemoryL2DecayWorker {
	if interval <= 0 {
		interval = memoryL2DecayDefaultInterval
	}
	return &MemoryL2DecayWorker{
		interval: interval,
		decayer:  decayer,
		agents:   agents,
		log:      log.NewHelper(logger),
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
				event.SysLogWarn("memory.l2_decay", "L2 maintenance target list failed", event.P("error", err))
				if w.log != nil {
					w.log.Warnf("memory l2 decay: list targets: %v", err)
				}
				return
			}
			decayCutoff := time.Now().UTC().Add(-memoryL2DecayMinEpisodeAge).Format(time.RFC3339Nano)
			var decayed, purged int
			for _, t := range targets {
				if !t.WriteL2Episode {
					continue
				}
				if n, err := w.decayer.ApplyEpisodeImportanceDecay(ctx, t.AgentID, decayCutoff, memoryL2DecayBatchFactor); err != nil {
					event.SysLogWarn("memory.l2_decay", "L2 episode importance decay failed", event.P("agent_id", t.AgentID), event.P("error", err))
				} else {
					decayed += n
				}
				retentionDays := t.L2RetentionDays
				if retentionDays <= 0 {
					retentionDays = 90
				}
				purgeCutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).Format(time.RFC3339Nano)
				if n, err := w.decayer.PurgeEpisodesOlderThan(ctx, t.AgentID, purgeCutoff); err != nil {
					event.SysLogWarn("memory.l2_decay", "L2 episode retention purge failed", event.P("agent_id", t.AgentID), event.P("error", err))
				} else {
					purged += n
				}
			}
			if (decayed > 0 || purged > 0) && w.log != nil {
				w.log.Infof("memory l2 decay: importance=%d purged=%d agents=%d", decayed, purged, len(targets))
			}
			return
		}
		cutoff := time.Now().UTC().Add(-memoryL2DecayMinEpisodeAge).Format(time.RFC3339Nano)
		n, err := w.decayer.ApplyAllEpisodeImportanceDecay(ctx, cutoff, memoryL2DecayBatchFactor)
		if err != nil {
			event.SysLogWarn("memory.l2_decay", "L2 episode importance decay failed", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("memory l2 decay: %v", err)
			}
			return
		}
		if n > 0 && w.log != nil {
			w.log.Infof("memory l2 decay: updated %d episodes (cutoff=%s factor=%v)", n, cutoff, memoryL2DecayBatchFactor)
		}
	})
}

func MemoryL2DecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L2_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
