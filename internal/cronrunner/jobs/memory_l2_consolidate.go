package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const memoryL2ConsolidateDefaultInterval = 10 * time.Minute

// MemoryL2ConsolidateWorker consolidates pending L2 episodes into L3 facts.
type MemoryL2ConsolidateWorker struct {
	interval time.Duration
	admin    *biz.MemoryAdminUsecase
	lg       loggateway.Logger
}

func NewMemoryL2ConsolidateWorker(interval time.Duration, admin *biz.MemoryAdminUsecase, lg loggateway.Logger) *MemoryL2ConsolidateWorker {
	if interval <= 0 {
		interval = memoryL2ConsolidateDefaultInterval
	}
	return &MemoryL2ConsolidateWorker{
		interval: interval,
		admin:    admin,
		lg:       lg,
	}
}

func (w *MemoryL2ConsolidateWorker) Start(ctx context.Context) {
	if w == nil || w.admin == nil {
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

func (w *MemoryL2ConsolidateWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.l2_consolidate", func() {
		rows, err := w.admin.ListPendingConsolidationEpisodes(ctx, "", 20)
		if err != nil {
			w.lg.Warn("L2 consolidation pending episode scan failed", loggateway.Err(err))
			return
		}
		if len(rows) == 0 {
			return
		}
		consolidated := 0
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			id := jsonutil.IfaceStr(m, "id")
			if id == "" {
				continue
			}
			// Mark as consolidated with zero extracted counts. The actual LLM-based fact
			// extraction is handled by AutoMemoryWorker (which already processes episodes
			// and creates L3/L4 artifacts). This worker ensures pending episodes transition
			// to consolidated status so they become visible in L2 recall queries.
			if err := w.admin.MarkEpisodeConsolidated(ctx, id, 0, 0); err != nil {
				w.lg.Warn("failed to consolidate episode",
					loggateway.Str("episode_id", id),
					loggateway.Err(err))
				continue
			}
			consolidated++
		}
		if consolidated > 0 {
			w.lg.Info("L2 consolidation batch done",
				loggateway.Int("consolidated", consolidated))
		}
	})
}

func (w *MemoryL2ConsolidateWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.admin == nil {
		return nil
	}
	rows, err := w.admin.ListPendingConsolidationEpisodes(ctx, "", 20)
	if err != nil {
		return fmt.Errorf("list pending episodes: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	consolidated := 0
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		id := jsonutil.IfaceStr(m, "id")
		if id == "" {
			continue
		}
		if err := w.admin.MarkEpisodeConsolidated(ctx, id, 0, 0); err != nil {
			w.lg.Warn("failed to consolidate episode",
				loggateway.Str("episode_id", id),
				loggateway.Err(err))
			continue
		}
		consolidated++
	}
	if consolidated > 0 {
		w.lg.Info("L2 consolidation batch done",
			loggateway.Int("consolidated", consolidated))
	}
	return nil
}

func MemoryL2ConsolidateDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L2_CONSOLIDATE_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
