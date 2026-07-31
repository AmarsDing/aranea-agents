package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// SelfImproveObserveWorker is the platform self-improvement Observe-stage
// scheduler (73-self-iteration-v3, design §5 self_improve_observe, 15min).
// Each tick delegates to SelfImprovementObserveUsecase.ScanOnce: trigger scan
// (4 signal triggers via the unified orchestrator) + run materialization for
// pending platform suggestions.
//
// The worker is only assembled when self_improvement.enabled=true (wire
// provider returns nil otherwise); the scan itself is idempotent.
type SelfImproveObserveWorker struct {
	interval time.Duration
	observe  *biz.SelfImprovementObserveUsecase
	lg       loggateway.Logger
}

// NewSelfImproveObserveWorker creates the worker. interval <= 0 defaults to
// 15 minutes (design §5).
func NewSelfImproveObserveWorker(
	interval time.Duration,
	observe *biz.SelfImprovementObserveUsecase,
	lg loggateway.Logger,
) *SelfImproveObserveWorker {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &SelfImproveObserveWorker{interval: interval, observe: observe, lg: lg}
}

// Start begins the worker loop. Blocks until ctx is cancelled.
func (w *SelfImproveObserveWorker) Start(ctx context.Context) {
	if w == nil || w.observe == nil {
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

func (w *SelfImproveObserveWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "self_improve.observe", func() {
		created, err := w.observe.ScanOnce(ctx)
		if err != nil {
			w.lg.Warn("self-improve observe worker: scan failed",
				loggateway.StepID("si_observe_worker.scan"),
				loggateway.Err(err))
			return
		}
		if created > 0 {
			w.lg.Info("self-improve observe worker: runs materialized",
				loggateway.StepID("si_observe_worker.scan"),
				loggateway.Int("created", created))
		}
	})
}
