package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// CuratorWorker periodically scans skills and runs the Curator Agent
// semi-automatic evolution pipeline: trigger detection → draft generation
// → sandbox verification → lifecycle update.
type CuratorWorker struct {
	interval time.Duration
	uc       *biz.SkillIntelligenceUsecase
	skills   biz.SkillQueryReader
	lg       loggateway.Logger
}

// NewCuratorWorker creates a CuratorWorker. If interval <= 0, defaults to 2 hours.
func NewCuratorWorker(interval time.Duration, uc *biz.SkillIntelligenceUsecase, skills biz.SkillQueryReader, lg loggateway.Logger) *CuratorWorker {
	if interval <= 0 {
		interval = 2 * time.Hour
	}
	return &CuratorWorker{interval: interval, uc: uc, skills: skills, lg: lg}
}

// Start begins the worker loop. Blocks until ctx is cancelled.
func (w *CuratorWorker) Start(ctx context.Context) {
	if w == nil || w.uc == nil || w.skills == nil {
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

func (w *CuratorWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "skill.curator", func() {
		if err := w.scanAll(ctx); err != nil {
			w.lg.Warn("curator worker scan failed", loggateway.StepID("skill.curator.scan"), loggateway.Err(err))
		}
	})
}

// scanAll iterates over all active skills and runs the Curator flow for each.
func (w *CuratorWorker) scanAll(ctx context.Context) error {
	const batchSize = 100
	offset := 0
	var errs int

	for {
		results, err := w.skills.SearchSkills(ctx, biz.SkillListQuery{
			Limit:  batchSize,
			Offset: offset,
			Status: "active",
		})
		if err != nil {
			return err
		}

		for _, skill := range results.Items {
			if _, flowErr := w.uc.RunCuratorFlow(ctx, skill.ID); flowErr != nil {
				w.lg.Warn("curator flow failed for skill",
					loggateway.StepID("skill.curator.flow"),
					loggateway.Str("skill_id", skill.ID),
					loggateway.Err(flowErr))
				errs++
			}
		}

		if len(results.Items) < batchSize {
			break
		}
		offset += batchSize
	}

	if errs > 0 {
		w.lg.Warn("curator worker completed with errors",
			loggateway.StepID("skill.curator.scan"),
			loggateway.Int("error_count", errs))
	}
	return nil
}
