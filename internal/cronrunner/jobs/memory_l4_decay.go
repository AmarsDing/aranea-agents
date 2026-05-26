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
	memoryL4DecayDefaultInterval = 24 * time.Hour
)

// MemoryL4DecayWorker periodically reduces confidence for stale L4 graph entities.
//
// MEM-OPT-02: runDecay inside L4GraphUsecase is only triggered inline during
// auto-memory writes, so entities that are read but never written never decay.
// This cron job drives decay globally across all agents that have L4 enabled,
// matching the per-agent schedule set in agent runtime settings.
type MemoryL4DecayWorker struct {
	interval time.Duration
	l4       biz.L4GraphWriter
	agents   *biz.AgentUsecase
	log      *log.Helper
}

// NewMemoryL4DecayWorker creates the L4 decay cron worker.
// Pass interval ≤ 0 for the default (24 h).
func NewMemoryL4DecayWorker(interval time.Duration, l4 biz.L4GraphWriter, agents *biz.AgentUsecase, logger log.Logger) *MemoryL4DecayWorker {
	if interval <= 0 {
		interval = memoryL4DecayDefaultInterval
	}
	return &MemoryL4DecayWorker{
		interval: interval,
		l4:       l4,
		agents:   agents,
		log:      log.NewHelper(logger),
	}
}

// Start blocks until ctx is cancelled, running L4 decay on each tick.
func (w *MemoryL4DecayWorker) Start(ctx context.Context) {
	if w == nil || w.l4 == nil {
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

func (w *MemoryL4DecayWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.l4_decay", func() {
		if w.agents == nil {
			return
		}
		targets, err := w.agents.ListMemoryMaintenanceTargets(ctx)
		if err != nil {
			event.SysLogWarn("memory.l4_decay", "L4 maintenance target list failed", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("memory l4 decay: list targets: %v", err)
			}
			return
		}
		var total int
		for _, t := range targets {
			if !t.WriteL4Graph {
				continue
			}
			w.l4.RunDecay(ctx, t.AgentID)
			total++
		}
		if total > 0 && w.log != nil {
			w.log.Debugf("memory l4 decay: triggered for %d agents", total)
		}
	})
}

// MemoryL4DecayDisabled returns true when the L4 decay cron is disabled via env.
func MemoryL4DecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L4_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
