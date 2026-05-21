package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// EvolutionScanner periodically scans agents for evolution metrics and creates suggestions.
type EvolutionScanner struct {
	interval time.Duration
	evo      *biz.EvolutionUsecase
	log      *log.Helper
}

// NewEvolutionScanner creates a scanner. Pass interval ≤0 for 30 minutes default.
func NewEvolutionScanner(interval time.Duration, evo *biz.EvolutionUsecase, logger log.Logger) *EvolutionScanner {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &EvolutionScanner{interval: interval, evo: evo, log: log.NewHelper(logger)}
}

// Start blocks until ctx is cancelled.
func (w *EvolutionScanner) Start(ctx context.Context) {
	if w == nil || w.evo == nil {
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

func (w *EvolutionScanner) runOnce(ctx context.Context) {
	safego.Go(ctx, "evolution.scanner", func() {
		if err := w.evo.ScanAll(ctx); err != nil && w.log != nil {
			w.log.Warnf("evolution scan: %v", err)
		}
	})
}
