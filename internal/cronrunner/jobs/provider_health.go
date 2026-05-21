package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// ProviderHealthScanner periodically checks LLM provider endpoint reachability.
type ProviderHealthScanner struct {
	interval time.Duration
	uc       *biz.LlmProviderModelUsecase
	log      *log.Helper
}

// NewProviderHealthScanner creates a scanner. Pass interval ≤0 for 5 minutes default.
func NewProviderHealthScanner(interval time.Duration, uc *biz.LlmProviderModelUsecase, logger log.Logger) *ProviderHealthScanner {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &ProviderHealthScanner{interval: interval, uc: uc, log: log.NewHelper(logger)}
}

// Start blocks until ctx is cancelled.
func (w *ProviderHealthScanner) Start(ctx context.Context) {
	if w == nil || w.uc == nil {
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

func (w *ProviderHealthScanner) runOnce(ctx context.Context) {
	safego.Go(ctx, "provider.health", func() {
		if err := w.uc.RunHealthChecks(ctx); err != nil && w.log != nil {
			w.log.Warnf("provider health: %v", err)
		}
	})
}
