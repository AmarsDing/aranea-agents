package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// EventStoreCleanup periodically purges expired event_store rows.
type EventStoreCleanup struct {
	interval time.Duration
	store    *biz.EventStoreUsecase
	lg       loggateway.Logger
}

func NewEventStoreCleanup(interval time.Duration, store *biz.EventStoreUsecase, lg loggateway.Logger) *EventStoreCleanup {
	if interval <= 0 {
		interval = time.Hour
	}
	return &EventStoreCleanup{
		interval: interval,
		store:    store,
		lg:       lg,
	}
}

func (w *EventStoreCleanup) Start(ctx context.Context) {
	if w == nil || w.store == nil {
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

func (w *EventStoreCleanup) runOnce(ctx context.Context) {
	safego.Go(ctx, "event_store.cleanup", func() {
		n, err := w.store.PurgeExpired(ctx)
		if err != nil {
			w.lg.Warn("event store cleanup failed", loggateway.Err(err))
			return
		}
		if n > 0 {
			w.lg.Info("event store cleanup removed rows", loggateway.Int("count", int(n)))
		}
	})
}

func EventStoreCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("EVENT_STORE_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
