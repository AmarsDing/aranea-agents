package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// EventStoreCleanup periodically purges expired event_store rows.
type EventStoreCleanup struct {
	interval time.Duration
	store    *biz.EventStoreUsecase
	log      *log.Helper
}

func NewEventStoreCleanup(interval time.Duration, store *biz.EventStoreUsecase, logger log.Logger) *EventStoreCleanup {
	if interval <= 0 {
		interval = time.Hour
	}
	return &EventStoreCleanup{
		interval: interval,
		store:    store,
		log:      log.NewHelper(logger),
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
			event.SysLogWarn("event_store.cleanup", "事件存储 TTL 清理失败", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("event store cleanup: %v", err)
			}
			return
		}
		if n > 0 && w.log != nil {
			w.log.Infof("event store cleanup: removed %d rows", n)
		}
	})
}

func EventStoreCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("EVENT_STORE_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
