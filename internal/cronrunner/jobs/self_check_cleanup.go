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

// SelfCheckCleanup periodically deletes old self-check reports.
type SelfCheckCleanup struct {
	interval time.Duration
	repo     bizmonitor.SelfCheckReportRepo
	lg       loggateway.Logger
}

// NewSelfCheckCleanup creates a new cleanup worker.
func NewSelfCheckCleanup(interval time.Duration, repo bizmonitor.SelfCheckReportRepo, lg loggateway.Logger) *SelfCheckCleanup {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &SelfCheckCleanup{
		interval: interval,
		repo:     repo,
		lg:       lg,
	}
}

// Start runs the cleanup loop until ctx is cancelled.
func (w *SelfCheckCleanup) Start(ctx context.Context) {
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

func (w *SelfCheckCleanup) runOnce(ctx context.Context) {
	safego.Go(ctx, "self_check.cleanup", func() {
		cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
		n, err := w.repo.DeleteSelfCheckReportsOlderThan(ctx, cutoff)
		if err != nil {
			w.lg.Warn("self-check report cleanup failed", loggateway.Err(err))
			return
		}
		if n > 0 {
			w.lg.Info("self-check report cleanup removed rows", loggateway.Int("count", int(n)))
		}
	})
}

// SelfCheckCleanupDisabled returns true if the SELF_CHECK_CLEANUP_DISABLED env var is set.
func SelfCheckCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("SELF_CHECK_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
