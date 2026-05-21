package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// ChannelHealthScanner periodically re-evaluates enabled channel configuration.
type ChannelHealthScanner struct {
	interval time.Duration
	uc       *biz.ChannelUsecase
	log      *log.Helper
}

// NewChannelHealthScanner creates a scanner. Pass interval ≤0 for 10 minutes default.
func NewChannelHealthScanner(interval time.Duration, uc *biz.ChannelUsecase, logger log.Logger) *ChannelHealthScanner {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &ChannelHealthScanner{interval: interval, uc: uc, log: log.NewHelper(logger)}
}

// Start blocks until ctx is cancelled.
func (w *ChannelHealthScanner) Start(ctx context.Context) {
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

func (w *ChannelHealthScanner) runOnce(ctx context.Context) {
	safego.Go(ctx, "channel.health", func() {
		if err := w.uc.RunHealthChecks(ctx); err != nil && w.log != nil {
			w.log.Warnf("channel health: %v", err)
		}
	})
}
