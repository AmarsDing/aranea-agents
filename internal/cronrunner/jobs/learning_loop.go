package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

type LearningLoopScanner struct {
	interval time.Duration
	loop     *biz.LearningLoopUsecase
	lg       loggateway.Logger
}

func NewLearningLoopScanner(interval time.Duration, loop *biz.LearningLoopUsecase, lg loggateway.Logger) *LearningLoopScanner {
	if interval <= 0 {
		interval = 60 * time.Minute
	}
	return &LearningLoopScanner{interval: interval, loop: loop, lg: lg}
}

func (w *LearningLoopScanner) Start(ctx context.Context) {
	if w == nil || w.loop == nil {
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

func (w *LearningLoopScanner) runOnce(ctx context.Context) {
	safego.Go(ctx, "learning.loop", func() {
		if err := w.loop.RunLoopAll(ctx); err != nil {
			w.lg.Warn("learning loop", loggateway.Err(err))
		}
	})
}
