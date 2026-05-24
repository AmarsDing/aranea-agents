package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	memoryL2DecayDefaultInterval = 24 * time.Hour
	memoryL2DecayMinEpisodeAge   = 24 * time.Hour
	memoryL2DecayBatchFactor     = 0.95
)

// MemoryL2DecayWorker periodically reduces stored episode importance for stale L2 rows.
type MemoryL2DecayWorker struct {
	interval time.Duration
	store    *sessionmemory.Store
	log      *log.Helper
}

func NewMemoryL2DecayWorker(interval time.Duration, store *sessionmemory.Store, logger log.Logger) *MemoryL2DecayWorker {
	if interval <= 0 {
		interval = memoryL2DecayDefaultInterval
	}
	return &MemoryL2DecayWorker{
		interval: interval,
		store:    store,
		log:      log.NewHelper(logger),
	}
}

func (w *MemoryL2DecayWorker) Start(ctx context.Context) {
	if w == nil || w.store == nil {
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
		cutoff := time.Now().UTC().Add(-memoryL2DecayMinEpisodeAge).Format(time.RFC3339Nano)
		n, err := w.store.ApplyAllEpisodeImportanceDecay(ctx, cutoff, memoryL2DecayBatchFactor)
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
