package jobs

import (
	"context"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

type PendingDeliveryProcessor interface {
	ProcessPending(ctx context.Context, limit int) error
}

type ChannelDeliveryWorker struct {
	interval time.Duration
	worker   PendingDeliveryProcessor
	lg       loggateway.Logger
	flowLog  biz.FlowLogWriter
	// running enforces single-flight inside one process so overlapping ticks
	// do not process the same claimed batch twice. Cross-replica exclusivity
	// is ClaimPendingDeliveries (SKIP LOCKED + sending lease), not this flag.
	running atomic.Bool
}

func NewChannelDeliveryWorker(interval time.Duration, worker PendingDeliveryProcessor, lg loggateway.Logger, flowLog biz.FlowLogWriter) *ChannelDeliveryWorker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &ChannelDeliveryWorker{interval: interval, worker: worker, lg: lg, flowLog: flowLog}
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
		w.processOnce(ctx)
	})
}

func (w *ChannelDeliveryWorker) processOnce(ctx context.Context) {
	if !w.running.CompareAndSwap(false, true) {
		return
	}
	defer w.running.Store(false)
	if err := w.worker.ProcessPending(ctx, 50); err != nil {
		w.lg.Warn("channel delivery failed",
			loggateway.StepID("system.channel_delivery.failed"),
			loggateway.Err(err))
		if w.flowLog != nil {
			w.flowLog.LogFlowError(context.Background(), "", "system.channel_delivery.failed",
				"渠道投递失败", biz.LogPair{Key: "error", Value: err.Error()})
		}
	}
}
