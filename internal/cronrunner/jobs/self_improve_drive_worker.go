package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// SelfImproveDriveWorker is the platform self-improvement full-chain drive
// scheduler (73-self-iteration-v3, Phase 4 W2b). Each tick delegates to
// SelfImprovementDriveUsecase.DriveOnce:
//
//	detected → Meta Team pipeline（异步，活跃集防重入）
//	陈旧中途态（崩溃孤儿 / pause 超时）→ recover 回 detected 重驱动
//	awaiting_governance → 治理路由（approval 通道每 run 每进程一次）
//	applying → Applier 重驱动；applied → PromoteEligible 补观察窗空位
//
// The worker is only assembled when self_improvement.enabled=true (wire
// provider returns nil otherwise); DriveOnce is idempotent and CAS-guarded.
type SelfImproveDriveWorker struct {
	interval time.Duration
	drive    *biz.SelfImprovementDriveUsecase
	lg       loggateway.Logger
}

// NewSelfImproveDriveWorker creates the worker. interval <= 0 defaults to
// 1 minute (drive prompts the pipeline promptly; the heavy work is async).
func NewSelfImproveDriveWorker(
	interval time.Duration,
	drive *biz.SelfImprovementDriveUsecase,
	lg loggateway.Logger,
) *SelfImproveDriveWorker {
	if interval <= 0 {
		interval = time.Minute
	}
	return &SelfImproveDriveWorker{interval: interval, drive: drive, lg: lg}
}

// Start begins the worker loop. Blocks until ctx is cancelled.
func (w *SelfImproveDriveWorker) Start(ctx context.Context) {
	if w == nil || w.drive == nil {
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

func (w *SelfImproveDriveWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "self_improve.drive", func() {
		if err := w.drive.DriveOnce(ctx); err != nil {
			w.lg.Warn("self-improve drive worker: tick failed",
				loggateway.StepID("si_drive_worker.tick"),
				loggateway.Err(err))
		}
	})
}
