package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// AutoHealTTLCleanup periodically removes resolved heal records that are
// older than the configured TTL. This prevents the heal_records table from
// growing unboundedly while preserving recent records for observability.
//
// Configurable via environment variables:
//
//	AUTO_HEAL_TTL_INTERVAL   — cleanup tick interval (e.g. "30m", "2h"); default 1h
//	AUTO_HEAL_TTL_MAX_AGE    — records older than this are deleted (e.g. "168h" for 7 days); default 72h
type AutoHealTTLCleanup struct {
	interval time.Duration
	maxAge   time.Duration
	repo     monitor.HealRecordRepo
	lg       loggateway.Logger
	log      *log.Helper
}

func defaultHealTTLInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("AUTO_HEAL_TTL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return time.Hour
}

func defaultHealTTLMaxAge() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("AUTO_HEAL_TTL_MAX_AGE")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 72 * time.Hour
}

// NewAutoHealTTLCleanup creates the cleanup worker.
// Pass interval/maxAge ≤ 0 to use environment-variable defaults.
func NewAutoHealTTLCleanup(interval, maxAge time.Duration, repo monitor.HealRecordRepo, lg loggateway.Logger, logger log.Logger) *AutoHealTTLCleanup {
	if interval <= 0 {
		interval = defaultHealTTLInterval()
	}
	if maxAge <= 0 {
		maxAge = defaultHealTTLMaxAge()
	}
	return &AutoHealTTLCleanup{
		interval: interval,
		maxAge:   maxAge,
		repo:     repo,
		lg:       lg,
		log:      log.NewHelper(logger),
	}
}

// Start runs the cleanup loop until ctx is cancelled.
func (c *AutoHealTTLCleanup) Start(ctx context.Context) {
	safego.Go(ctx, "auto_heal_ttl_cleanup", func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.runOnce(ctx)
			}
		}
	})
}

func (c *AutoHealTTLCleanup) runOnce(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-c.maxAge)
	deleted, err := c.repo.DeleteHealRecordsOlderThan(ctx, cutoff)
	if err != nil {
		c.lg.Error("AutoHealTTLCleanup: failed to delete old records",
			loggateway.StepID("auto_heal_ttl_cleanup_fail"),
			loggateway.Err(err))
		return
	}
	if deleted > 0 {
		c.lg.Info("AutoHealTTLCleanup: deleted old heal records",
			loggateway.StepID("auto_heal_ttl_cleanup"),
			loggateway.Int("deleted", deleted),
			loggateway.Int64("max_age_hours", int64(c.maxAge.Hours())))
	}
}
