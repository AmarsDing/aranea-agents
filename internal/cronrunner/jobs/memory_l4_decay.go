package jobs

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	memoryL4DecayDefaultInterval = 24 * time.Hour
	memoryL4DecayBatchSize       = 500
	memoryL4ArchiveThreshold     = 0.1
)

type MemoryL4DecayWorker struct {
	interval time.Duration
	l4       biz.L4GraphWriter
	agents   *biz.AgentUsecase
	cfg      biz.L4DecayConfig
	log      *log.Helper
}

func NewMemoryL4DecayWorker(interval time.Duration, l4 biz.L4GraphWriter, agents *biz.AgentUsecase, logger log.Logger) *MemoryL4DecayWorker {
	if interval <= 0 {
		interval = memoryL4DecayDefaultInterval
	}
	if envHours := os.Getenv("MEMORY_L4_DECAY_INTERVAL_HOURS"); envHours != "" {
		if h, err := strconv.Atoi(envHours); err == nil && h > 0 {
			interval = time.Duration(h) * time.Hour
		}
	}
	return &MemoryL4DecayWorker{
		interval: interval,
		l4:       l4,
		agents:   agents,
		cfg:      biz.DefaultL4DecayConfig(),
		log:      log.NewHelper(logger),
	}
}

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
		var totalDecayed, totalArchived, totalAgents int
		for _, t := range targets {
			if !t.WriteL4Graph {
				continue
			}
			result := w.l4.RunDecayWithConfig(ctx, t.AgentID, w.cfg)
			totalDecayed += result.Decayed
			totalArchived += result.Archived
			totalAgents++
		}
		if totalAgents > 0 && w.log != nil {
			w.log.Debugf("memory l4 decay: %d agents, %d decayed, %d archived", totalAgents, totalDecayed, totalArchived)
		}
	})
}

func MemoryL4DecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L4_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
