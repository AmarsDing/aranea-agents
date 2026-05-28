package jobs

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	memoryIndexReconcileDefaultInterval = 6 * time.Hour
	memoryIndexReconcileMaxAttempts     = 5
	memoryIndexReconcileBatchSize       = 200
)

type MemoryFactIndexReconciler struct {
	interval  time.Duration
	store     *sessionmemory.Store
	indexSync biz.MemoryFactIndexSyncer
	log       *log.Helper
}

func NewMemoryFactIndexReconciler(interval time.Duration, store *sessionmemory.Store, indexSync biz.MemoryFactIndexSyncer, logger log.Logger) *MemoryFactIndexReconciler {
	if interval <= 0 {
		interval = memoryIndexReconcileDefaultInterval
	}
	return &MemoryFactIndexReconciler{
		interval:  interval,
		store:     store,
		indexSync: indexSync,
		log:       log.NewHelper(logger),
	}
}

func (w *MemoryFactIndexReconciler) Start(ctx context.Context) {
	if w == nil || w.store == nil || w.indexSync == nil {
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

func (w *MemoryFactIndexReconciler) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.index_reconcile", func() {
		rows, err := w.store.ListStaleIndexFacts(ctx, memoryIndexReconcileMaxAttempts, memoryIndexReconcileBatchSize)
		if err != nil {
			event.SysLogWarn("memory.index_reconcile", "list stale facts failed", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("memory index reconcile: list stale: %v", err)
			}
			return
		}
		if len(rows) == 0 {
			return
		}
		var synced, failed, disabled int
		for _, raw := range rows {
			if err := w.indexSync.SyncFactIndexFromRow(ctx, raw); err != nil {
				failed++
				factID := extractFactIDFromRow(raw)
				if factID != "" {
					var row struct {
						IndexAttempts int `json:"index_attempts"`
					}
					if jsonErr := json.Unmarshal(raw, &row); jsonErr == nil && row.IndexAttempts >= memoryIndexReconcileMaxAttempts-1 {
						if disableErr := w.store.MarkFactIndexDisabled(ctx, factID); disableErr == nil {
							disabled++
							event.SysLogWarn("memory.index_reconcile", "fact index permanently disabled after max attempts",
								event.P("fact_id", factID), event.P("attempts", row.IndexAttempts))
						}
					}
				}
				continue
			}
			synced++
		}
		if w.log != nil {
			w.log.Infof("memory index reconcile: synced=%d failed=%d disabled=%d total=%d", synced, failed, disabled, len(rows))
		}
	})
}

func extractFactIDFromRow(raw []byte) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if id, ok := m["id"].(string); ok {
		return id
	}
	return ""
}

func MemoryIndexReconcileDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_INDEX_RECONCILE_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
