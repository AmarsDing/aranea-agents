package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// MonitorAlertCooldownCleanup periodically evicts stale lastFired entries from
// the in-process sync.Map used by MonitorUsecase.ShouldFireAlert.
//
// Without this cleanup, a process running for weeks/months accumulates entries
// for every alert rule ID that has ever fired, leaking memory and — after a
// process restart — starting every cooldown window fresh anyway.
//
// MON-05: wires CleanupStaleLastFired into the cron scheduler so it runs on
// every instance, once per hour, with a 24-hour max-age window.
//
// Configurable via environment variables:
//
//	MONITOR_ALERT_COOLDOWN_INTERVAL — cleanup tick interval (e.g. "30m", "2h"); default 1h
//	MONITOR_ALERT_COOLDOWN_MAX_AGE  — entries older than this are evicted (e.g. "48h"); default 24h
type MonitorAlertCooldownCleanup struct {
	interval time.Duration
	maxAge   time.Duration
	uc       *biz.MonitorUsecase
	log      *log.Helper
}

// defaultCooldownInterval returns the cleanup interval from the env or falls back to 1 hour.
func defaultCooldownInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MONITOR_ALERT_COOLDOWN_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return time.Hour
}

// defaultCooldownMaxAge returns the max-age window from the env or falls back to 24 hours.
func defaultCooldownMaxAge() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MONITOR_ALERT_COOLDOWN_MAX_AGE")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 24 * time.Hour
}

// NewMonitorAlertCooldownCleanup creates the cleanup worker.
// Pass interval/maxAge ≤ 0 to use environment-variable defaults (see type doc).
func NewMonitorAlertCooldownCleanup(interval, maxAge time.Duration, uc *biz.MonitorUsecase, logger log.Logger) *MonitorAlertCooldownCleanup {
	if interval <= 0 {
		interval = defaultCooldownInterval()
	}
	if maxAge <= 0 {
		maxAge = defaultCooldownMaxAge()
	}
	return &MonitorAlertCooldownCleanup{
		interval: interval,
		maxAge:   maxAge,
		uc:       uc,
		log:      log.NewHelper(logger),
	}
}

// Start blocks until ctx is cancelled, running cleanup on each tick.
func (w *MonitorAlertCooldownCleanup) Start(ctx context.Context) {
	if w == nil || w.uc == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.runOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce()
		}
	}
}

func (w *MonitorAlertCooldownCleanup) runOnce() {
	safego.Go(appctx.Ctx(), "monitor.alert_cooldown_cleanup", func() {
		w.uc.CleanupStaleLastFired(time.Now(), w.maxAge)
		if w.log != nil {
			w.log.Debugf("monitor alert cooldown cleanup ran (maxAge=%s)", w.maxAge)
		}
	})
}
