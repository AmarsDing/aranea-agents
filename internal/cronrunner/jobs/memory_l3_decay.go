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
	memoryL3DecayDefaultInterval = 24 * time.Hour
	memoryL3DecayMinFactAge      = 24 * time.Hour
	memoryL3DecayBatchFactor     = 0.97
)

// MemoryL3DecayWorker periodically reduces stored fact importance for stale L3 rows.
type MemoryL3DecayWorker struct {
	interval time.Duration
	store    *sessionmemory.Store
	log      *log.Helper
}

func NewMemoryL3DecayWorker(interval time.Duration, store *sessionmemory.Store, logger log.Logger) *MemoryL3DecayWorker {
	if interval <= 0 {
		interval = memoryL3DecayDefaultInterval
	}
	return &MemoryL3DecayWorker{
		interval: interval,
		store:    store,
		log:      log.NewHelper(logger),
	}
}

func (w *MemoryL3DecayWorker) Start(ctx context.Context) {
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

func (w *MemoryL3DecayWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.l3_decay", func() {
		cutoff := time.Now().UTC().Add(-memoryL3DecayMinFactAge).Format(time.RFC3339Nano)
		n, err := w.store.ApplyAllFactImportanceDecay(ctx, cutoff, memoryL3DecayBatchFactor)
		if err != nil {
			event.SysLogWarn("memory.l3_decay", "L3 fact importance decay failed", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("memory l3 decay: %v", err)
			}
			return
		}
		if n > 0 && w.log != nil {
			w.log.Infof("memory l3 decay: updated %d facts (cutoff=%s factor=%v)", n, cutoff, memoryL3DecayBatchFactor)
		}
	})
}

func MemoryL3DecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L3_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
