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

// SelfCheckJob runs the self-check scheduler on a periodic basis via CronRunner.
type SelfCheckJob struct {
	interval  time.Duration
	scheduler *bizmonitor.SelfCheckScheduler
	lg        loggateway.Logger
}

// NewSelfCheckJob creates a new self-check periodic job.
func NewSelfCheckJob(interval time.Duration, scheduler *bizmonitor.SelfCheckScheduler, lg loggateway.Logger) *SelfCheckJob {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &SelfCheckJob{
		interval:  interval,
		scheduler: scheduler,
		lg:        lg,
	}
}

// Start runs the self-check job loop until ctx is cancelled.
func (j *SelfCheckJob) Start(ctx context.Context) {
	if j == nil || j.scheduler == nil {
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

func (j *SelfCheckJob) runOnce(ctx context.Context) {
	safego.Go(ctx, "self_check.job", func() {
		j.scheduler.RunOnce(ctx)
	})
}

// SelfCheckJobDisabled returns true if the SELF_CHECK_JOB_DISABLED env var is set.
func SelfCheckJobDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("SELF_CHECK_JOB_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
