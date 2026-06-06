package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	bizmonitor "aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// PatternMiningJob runs the PatternMiningUsecase on a daily basis via CronRunner.
// It reads historical repair records, clusters similar failure modes, and
// generates mined fix templates for the failure_pattern knowledge base.
type PatternMiningJob struct {
	interval time.Duration
	uc       *bizmonitor.PatternMiningUsecase
	lg       loggateway.Logger
}

// NewPatternMiningJob creates a new pattern mining periodic job.
// Pass interval ≤ 0 to use the environment-variable default or 24 hours.
func NewPatternMiningJob(interval time.Duration, uc *bizmonitor.PatternMiningUsecase, lg loggateway.Logger) *PatternMiningJob {
	if interval <= 0 {
		interval = patternMiningIntervalFromEnv()
	}
	return &PatternMiningJob{
		interval: interval,
		uc:       uc,
		lg:       lg,
	}
}

func patternMiningIntervalFromEnv() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("PATTERN_MINING_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= time.Minute {
			return d
		}
	}
	return 24 * time.Hour
}

// Start runs the pattern mining job loop until ctx is cancelled.
func (j *PatternMiningJob) Start(ctx context.Context) {
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

func (j *PatternMiningJob) runOnce(ctx context.Context) {
	safego.Go(ctx, "pattern_mining.job", func() {
		result, err := j.uc.Mine(ctx)
		if err != nil {
			j.lg.Error("PatternMiningJob: Mine failed",
				loggateway.StepID("pattern_mining.job_fail"),
				loggateway.Err(err))
			return
		}
		if result.PatternsCreated > 0 || result.PatternsUpdated > 0 || result.PatternsDeactivated > 0 {
			j.lg.Info("PatternMiningJob: mining cycle complete",
				loggateway.StepID("pattern_mining.job_done"),
				loggateway.Int("clusters_analyzed", result.ClustersAnalyzed),
				loggateway.Int("created", result.PatternsCreated),
				loggateway.Int("updated", result.PatternsUpdated),
				loggateway.Int("deactivated", result.PatternsDeactivated))
		}
	})
}
