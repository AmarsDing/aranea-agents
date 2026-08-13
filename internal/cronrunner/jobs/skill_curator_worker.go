package jobs

import (
	"context"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const defaultCuratorDailyMax = 20

// curatorDailyMaxFromEnv reads CURATOR_DAILY_MAX from environment,
// falling back to defaultCuratorDailyMax (20) if unset or invalid.
func curatorDailyMaxFromEnv() int32 {
	if v := os.Getenv("CURATOR_DAILY_MAX"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			return int32(n)
		}
	}
	return defaultCuratorDailyMax
}

// CuratorWorker periodically scans skills and runs the verification half of
// the Curator pipeline: draft generation → sandbox verification → lifecycle
// update, for pending suggestions created by EvolutionOrchestratorWorker
// (the unified trigger entry, A1). It no longer triggers suggestions itself.
type CuratorWorker struct {
	interval   time.Duration
	uc         *biz.SkillIntelligenceUsecase
	skills     biz.SkillQueryReader
	lg         loggateway.Logger
	dailyMax   int32
	dailyCount atomic.Int32
	dailyReset atomic.Int64 // Unix timestamp of last daily reset
	running    atomic.Bool  // single-flight: skip a tick while the previous scan is still in flight
}

// NewCuratorWorker creates a CuratorWorker. If interval <= 0, defaults to 2 hours.
func NewCuratorWorker(interval time.Duration, uc *biz.SkillIntelligenceUsecase, skills biz.SkillQueryReader, lg loggateway.Logger) *CuratorWorker {
	if interval <= 0 {
		interval = 2 * time.Hour
	}
	return &CuratorWorker{interval: interval, uc: uc, skills: skills, lg: lg, dailyMax: curatorDailyMaxFromEnv()}
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
		if !w.running.CompareAndSwap(false, true) {
			w.lg.Warn("curator worker: previous scan still running, skipping tick",
				loggateway.StepID("skill.curator.skip"))
			return
		}
		defer w.running.Store(false)
		// Expiration of stale pending suggestions is owned by
		// EvolutionOrchestratorWorker (orchestrator.ExpirePending → status
		// 'expired'); the curator only runs the verification half.
		if err := w.scanAll(ctx); err != nil {
			w.lg.Warn("curator worker scan failed", loggateway.StepID("skill.curator.scan"), loggateway.Err(err))
		}
	})
}

// scanAll iterates over all active skills and validates their orchestrator-
// created pending suggestions (draft + sandbox + lifecycle). It enforces a
// daily maximum of curatorDailyMax (20) flow invocations.
func (w *CuratorWorker) scanAll(ctx context.Context) error {
	// Reset daily counter if a new day has started.
	now := time.Now()
	resetTime := time.Unix(w.dailyReset.Load(), 0)
	if now.Sub(resetTime) >= 24*time.Hour {
		w.dailyCount.Store(0)
		w.dailyReset.Store(now.Unix())
	}

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
			if w.dailyCount.Load() >= w.dailyMax {
				w.lg.Info("curator daily limit reached, skipping remaining skills",
					loggateway.StepID("skill.curator.scan"),
					loggateway.Int("daily_limit", int(w.dailyMax)))
				return nil
			}
			if flowErr := w.uc.ValidatePendingSuggestionsForSkill(ctx, skill.ID); flowErr != nil {
				w.lg.Warn("curator validation failed for skill",
					loggateway.StepID("skill.curator.validate"),
					loggateway.Str("skill_id", skill.ID),
					loggateway.Err(flowErr))
				errs++
			} else {
				w.dailyCount.Add(1)
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
