package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/service"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// ChannelDeliveryWorker drains pending outbound channel messages with retry.
type ChannelDeliveryWorker struct {
	interval time.Duration
	worker   *service.ChannelDeliveryWorker
	log      *log.Helper
}

// NewChannelDeliveryWorker creates a worker. Pass interval ≤0 for 5 seconds default.
func NewChannelDeliveryWorker(interval time.Duration, worker *service.ChannelDeliveryWorker, logger log.Logger) *ChannelDeliveryWorker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &ChannelDeliveryWorker{interval: interval, worker: worker, log: log.NewHelper(logger)}
}

// Start blocks until ctx is cancelled.
func (w *ChannelDeliveryWorker) Start(ctx context.Context) {
	if w == nil || w.worker == nil {
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

func (w *ChannelDeliveryWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "channel.delivery", func() {
		if err := w.worker.ProcessPending(ctx, 50); err != nil && w.log != nil {
			w.log.Warnf("channel delivery: %v", err)
		}
	})
}
