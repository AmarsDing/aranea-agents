package jobs

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	memoryL4DecayDefaultInterval = 24 * time.Hour
	memoryL4ArchiveThreshold     = 0.1
)

type MemoryL4DecayWorker struct {
	interval time.Duration
	l4       biz.L4GraphWriter
	agents   *biz.AgentUsecase
	cfg      biz.L4DecayConfig
	lg       loggateway.Logger
}

func NewMemoryL4DecayWorker(interval time.Duration, l4 biz.L4GraphWriter, agents *biz.AgentUsecase, lg loggateway.Logger) *MemoryL4DecayWorker {
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
		lg:       lg,
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
			w.lg.Warn("L4 maintenance target list failed", loggateway.Err(err))
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
		if totalAgents > 0 {
			w.lg.Debug("memory l4 decay completed", loggateway.Int("agents", totalAgents), loggateway.Int("decayed", totalDecayed), loggateway.Int("archived", totalArchived))
		}
	})
}

func MemoryL4DecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L4_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
