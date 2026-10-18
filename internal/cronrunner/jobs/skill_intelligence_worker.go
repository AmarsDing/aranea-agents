package jobs

import (
	"context"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// SkillIntelligenceWorker periodically scans recent skill invocations,
// generates experience reports for invocations that don't have one yet.
type SkillIntelligenceWorker struct {
	interval time.Duration
	uc       *biz.SkillIntelligenceUsecase
	lg       loggateway.Logger
	// 审计 W4：单次扫描超时过 interval 时，
	// 无守卫会并发叠加多份扫描（幂等但浪费 CPU/DB），原子标志防重入。
	inFlight atomic.Bool
}

// NewSkillIntelligenceWorker creates a new SkillIntelligenceWorker.
// If interval <= 0, defaults to 15 minutes.
func NewSkillIntelligenceWorker(interval time.Duration, uc *biz.SkillIntelligenceUsecase, lg loggateway.Logger) *SkillIntelligenceWorker {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &SkillIntelligenceWorker{interval: interval, uc: uc, lg: lg}
}

// Start begins the worker loop. Blocks until ctx is cancelled.
func (w *SkillIntelligenceWorker) Start(ctx context.Context) {
	if w == nil || w.uc == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	// Run once immediately on start.
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

func (w *SkillIntelligenceWorker) runOnce(ctx context.Context) {
	// Skip this tick if a previous scan is still running (审计 W4 防重入)。
	if !w.inFlight.CompareAndSwap(false, true) {
		w.lg.Info("skill intelligence skip tick: previous scan still in-flight",
			loggateway.StepID("skill_intelligence.worker"))
		return
	}
	safego.Go(ctx, "skill.intelligence", func() {
		defer w.inFlight.Store(false)
		// Scan recent sessions with skill invocations and generate reports.
		// The Usecase method handles the actual scanning logic.
		if err := w.uc.ScanAndGenerateReports(ctx); err != nil {
			w.lg.Warn("skill intelligence scan failed", loggateway.Err(err))
		}
	})
}
