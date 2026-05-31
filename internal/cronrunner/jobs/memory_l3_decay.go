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
	memoryL3DecayDefaultInterval = 24 * time.Hour
	memoryL3DecayBatchFactor     = 0.97
)

// MemoryL3DecayWorker periodically reduces stored fact importance for stale L3 rows.
type MemoryL3DecayWorker struct {
	interval time.Duration
	decayer  biz.MemoryFactDecayer
	agents   *biz.AgentUsecase
	lg       loggateway.Logger
}

func NewMemoryL3DecayWorker(interval time.Duration, decayer biz.MemoryFactDecayer, agents *biz.AgentUsecase, lg loggateway.Logger) *MemoryL3DecayWorker {
	if interval <= 0 {
		interval = memoryL3DecayDefaultInterval
	}
	return &MemoryL3DecayWorker{
		interval: interval,
		decayer:  decayer,
		agents:   agents,
		lg:       lg,
	}
}

func (w *MemoryL3DecayWorker) Start(ctx context.Context) {
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

func (w *MemoryL3DecayWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.l3_decay", func() {
		if w.agents != nil {
			targets, err := w.agents.ListMemoryMaintenanceTargets(ctx)
			if err != nil {
				w.lg.Warn("L3 maintenance target list failed", loggateway.Err(err))
				return
			}
			var total int
			for _, t := range targets {
				if !t.WriteL3Facts {
					continue
				}
				intervalHours := t.L3DecayIntervalHours
				if intervalHours <= 0 {
					intervalHours = 24
				}
				cutoff := time.Now().UTC().Add(-time.Duration(intervalHours) * time.Hour).Format(time.RFC3339Nano)
				n, err := w.decayer.ApplyAgentFactImportanceDecay(ctx, t.AgentID, cutoff, memoryL3DecayBatchFactor)
				if err != nil {
					w.lg.Warn("L3 fact importance decay failed", loggateway.Str("agent_id", t.AgentID), loggateway.Err(err))
					continue
				}
				total += n
			}
			if total > 0 {
				w.lg.Info("memory l3 decay updated facts", loggateway.Int("count", total), loggateway.Int("agents", len(targets)))
			}
			return
		}
		cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
		n, err := w.decayer.ApplyAllFactImportanceDecay(ctx, cutoff, memoryL3DecayBatchFactor)
		if err != nil {
			w.lg.Warn("L3 fact importance decay failed", loggateway.Err(err))
			return
		}
		if n > 0 {
			w.lg.Info("memory l3 decay updated facts", loggateway.Int("count", n), loggateway.Str("cutoff", cutoff), loggateway.Any("factor", memoryL3DecayBatchFactor))
		}
	})
}

func MemoryL3DecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L3_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
