package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// SelfImproveOutcomeWorker is the platform self-improvement Learn-stage
// scheduler (73-self-iteration-v3, design §5 self_improve_outcome, 1h).
// Each tick delegates to SelfImprovementOutcomeUsecase.ScanOnce: 终态 run
// 生成 PatchOutcome 归因 + regressed 写 KB 负面样本 + 触发器自适应降频。
//
// The worker is only assembled when self_improvement.enabled=true (wire
// provider returns nil otherwise); attribution is idempotent
// (ListTerminalPendingOutcome 只取尚无 outcome 的终态 run)。
type SelfImproveOutcomeWorker struct {
	interval time.Duration
	outcome  *biz.SelfImprovementOutcomeUsecase
	lg       loggateway.Logger
}

// NewSelfImproveOutcomeWorker creates the worker. interval <= 0 defaults to
// 1 hour (design §5).
func NewSelfImproveOutcomeWorker(
	interval time.Duration,
	outcome *biz.SelfImprovementOutcomeUsecase,
	lg loggateway.Logger,
) *SelfImproveOutcomeWorker {
	if interval <= 0 {
		interval = time.Hour
	}
	return &SelfImproveOutcomeWorker{interval: interval, outcome: outcome, lg: lg}
}

// Start begins the worker loop. Blocks until ctx is cancelled.
func (w *SelfImproveOutcomeWorker) Start(ctx context.Context) {
	if w == nil || w.outcome == nil {
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

func (w *SelfImproveOutcomeWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "self_improve.outcome", func() {
		if err := w.outcome.ScanOnce(ctx); err != nil {
			w.lg.Warn("self-improve outcome worker: scan failed",
				loggateway.StepID("si_outcome_worker.scan"),
				loggateway.Err(err))
		}
	})
}
