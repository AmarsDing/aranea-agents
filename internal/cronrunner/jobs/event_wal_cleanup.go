package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const defaultEventWALTTL = 7 * 24 * time.Hour

// EventWALCleanup periodically purges published entries from the event_wal table.
// This prevents unbounded growth of the WAL while preserving unpublished entries
// for crash recovery (AS-EVT-01).
type EventWALCleanup struct {
	interval time.Duration
	ttl      time.Duration
	wal      *event.EventWAL
	lg       loggateway.Logger
}

// NewEventWALCleanup creates a WAL cleanup worker.
// If interval <= 0, defaults to 1 hour. If ttl <= 0, defaults to 7 days.
func NewEventWALCleanup(interval, ttl time.Duration, wal *event.EventWAL, lg loggateway.Logger) *EventWALCleanup {
	if interval <= 0 {
		interval = time.Hour
	}
	if ttl <= 0 {
		ttl = defaultEventWALTTL
	}
	return &EventWALCleanup{
		interval: interval,
		ttl:      ttl,
		wal:      wal,
		lg:       lg,
	}
}

// Start runs the cleanup loop until ctx is cancelled.
func (w *EventWALCleanup) Start(ctx context.Context) {
	if w == nil || w.wal == nil {
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

func (w *EventWALCleanup) runOnce(ctx context.Context) {
	safego.Go(ctx, "event_wal.cleanup", func() {
		n, err := w.wal.PurgePublished(ctx, w.ttl)
		if err != nil {
			w.lg.Warn("event WAL cleanup failed", loggateway.Err(err))
			return
		}
		if n > 0 {
			w.lg.Info("event WAL cleanup removed published entries", loggateway.Int("count", int(n)))
		}
	})
}

// EventWALCleanupDisabled reports whether WAL cleanup is disabled via env var.
func EventWALCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("EVENT_WAL_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
