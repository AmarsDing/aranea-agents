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

// MonitorEventsRetention is the retention window for monitor_events rows.
// 30 天与 self_check_reports 一致；告警窗口/Runner 指标按分钟-小时聚合，
// Runs 真相源为 monitor_traces（OPT-05），更长保留无业务消费方。
const MonitorEventsRetention = 30 * 24 * time.Hour

// MonitorEventsCleanup periodically deletes old monitor_events rows so the
// Events 历史表（含 runner.completion 每对话双写）不会无限增长。
type MonitorEventsCleanup struct {
	interval time.Duration
	repo     bizmonitor.EventRepo
	lg       loggateway.Logger
}

// NewMonitorEventsCleanup creates a new cleanup worker.
func NewMonitorEventsCleanup(interval time.Duration, repo bizmonitor.EventRepo, lg loggateway.Logger) *MonitorEventsCleanup {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &MonitorEventsCleanup{
		interval: interval,
		repo:     repo,
		lg:       lg,
	}
}

// Start runs the cleanup loop until ctx is cancelled.
func (w *MonitorEventsCleanup) Start(ctx context.Context) {
	if w == nil || w.repo == nil {
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

func (w *MonitorEventsCleanup) runOnce(ctx context.Context) {
	safego.Go(ctx, "monitor_events.cleanup", func() {
		cutoff := time.Now().UTC().Add(-MonitorEventsRetention)
		n, err := w.repo.DeleteMonitorEventsOlderThan(ctx, cutoff)
		if err != nil {
			w.lg.Warn("monitor events cleanup failed", loggateway.Err(err))
			return
		}
		if n > 0 {
			w.lg.Info("monitor events cleanup removed rows", loggateway.Int("count", n))
		}
	})
}

// MonitorEventsCleanupDisabled returns true if the MONITOR_EVENTS_CLEANUP_DISABLED env var is set.
func MonitorEventsCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MONITOR_EVENTS_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
