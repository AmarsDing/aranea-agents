package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

type SkillEvolutionScanner struct {
	interval time.Duration
	skillEvo *biz.SkillEvolutionUsecase
	lg       loggateway.Logger
}

func NewSkillEvolutionScanner(interval time.Duration, skillEvo *biz.SkillEvolutionUsecase, lg loggateway.Logger) *SkillEvolutionScanner {
	if interval <= 0 {
		interval = 60 * time.Minute
	}
	return &SkillEvolutionScanner{interval: interval, skillEvo: skillEvo, lg: lg}
}

func (w *SkillEvolutionScanner) Start(ctx context.Context) {
	if w == nil || w.skillEvo == nil {
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

func (w *SkillEvolutionScanner) runOnce(ctx context.Context) {
	safego.Go(ctx, "skill.evolution", func() {
		if err := w.skillEvo.ScanAndProposeAll(ctx); err != nil {
			w.lg.Warn("skill evolution scan", loggateway.Err(err))
		}
	})
}
