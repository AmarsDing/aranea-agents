package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// SelfImproveWatchdogWorker is the platform self-improvement Watchdog-stage
// scheduler (73-self-iteration-v3, design §5 self_improve_watchdog, 5min).
// Each tick delegates to SelfImprovementWatchdogUsecase.ScanOnce: observing
// run 基线采集 → 到期指标对比 → close 或自动 rollback + 管理员通知。
//
// The worker is only assembled when self_improvement.enabled=true (wire
// provider returns nil otherwise); rollback 以 run 状态 CAS 防重。
type SelfImproveWatchdogWorker struct {
	interval time.Duration
	watchdog *biz.SelfImprovementWatchdogUsecase
	lg       loggateway.Logger
}

// NewSelfImproveWatchdogWorker creates the worker. interval <= 0 defaults to
// 5 minutes (design §5).
func NewSelfImproveWatchdogWorker(
	interval time.Duration,
	watchdog *biz.SelfImprovementWatchdogUsecase,
	lg loggateway.Logger,
) *SelfImproveWatchdogWorker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &SelfImproveWatchdogWorker{interval: interval, watchdog: watchdog, lg: lg}
}

// Start begins the worker loop. Blocks until ctx is cancelled.
func (w *SelfImproveWatchdogWorker) Start(ctx context.Context) {
	if w == nil || w.watchdog == nil {
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

func (w *SelfImproveWatchdogWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "self_improve.watchdog", func() {
		if err := w.watchdog.ScanOnce(ctx); err != nil {
			w.lg.Warn("self-improve watchdog worker: scan failed",
				loggateway.StepID("si_watchdog_worker.scan"),
				loggateway.Err(err))
		}
	})
}
