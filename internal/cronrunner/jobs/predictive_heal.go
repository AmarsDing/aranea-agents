package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// PredictiveHealJob runs the PredictiveHealUsecase on a periodic basis via CronRunner.
// It scans system metrics, matches precondition patterns from the FailurePattern
// knowledge base, and executes preventive actions when confidence exceeds 0.8.
type PredictiveHealJob struct {
	interval time.Duration
	uc       *heal.PredictiveHealUsecase
	lg       loggateway.Logger
}

// NewPredictiveHealJob creates a new predictive heal periodic job.
// Pass interval ≤ 0 to use the environment-variable default or 5 minutes.
func NewPredictiveHealJob(interval time.Duration, uc *heal.PredictiveHealUsecase, lg loggateway.Logger) *PredictiveHealJob {
	if interval <= 0 {
		interval = predictiveHealIntervalFromEnv()
	}
	return &PredictiveHealJob{
		interval: interval,
		uc:       uc,
		lg:       lg,
	}
}

func predictiveHealIntervalFromEnv() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("PREDICTIVE_HEAL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= time.Minute {
			return d
		}
	}
	return 5 * time.Minute
}

// PredictiveHealJobEnabled reports whether the predictive heal job should run.
// Default is ENABLED: the wired HealActionHandler is the real action catalog
// (CatalogHealActionHandler — provider/MCP health refresh probes, both
// idempotent read-only operations), and the confidence gate is metric-driven
// so actions only fire on real metric signals with a 30-minute per-type
// cooldown. Set PREDICTIVE_HEAL_JOB_ENABLED=0 to opt out.
func PredictiveHealJobEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("PREDICTIVE_HEAL_JOB_ENABLED")))
	return raw != "0" && raw != "false" && raw != "no"
}

// Start runs the predictive heal job loop until ctx is cancelled.
func (j *PredictiveHealJob) Start(ctx context.Context) {
	if j == nil || j.uc == nil {
		return
	}
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	// Run once immediately
	j.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.runOnce(ctx)
		}
	}
}

func (j *PredictiveHealJob) runOnce(ctx context.Context) {
	safego.Go(ctx, "predictive_heal.job", func() {
		records, err := j.uc.PredictAndHeal(ctx)
		if err != nil {
			j.lg.Error("PredictiveHealJob: PredictAndHeal failed",
				loggateway.StepID("predictive_heal.job_fail"),
				loggateway.Err(err))
			return
		}
		applied := 0
		skipped := 0
		for _, r := range records {
			if r.Status == string(heal.HealStatusApplied) {
				applied++
			} else {
				skipped++
			}
		}
		if len(records) > 0 {
			j.lg.Info("PredictiveHealJob: scan complete",
				loggateway.StepID("predictive_heal.job_done"),
				loggateway.Int("total", len(records)),
				loggateway.Int("applied", applied),
				loggateway.Int("skipped", skipped))
		}
	})
}
