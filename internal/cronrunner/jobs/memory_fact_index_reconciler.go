package jobs

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	memoryIndexReconcileDefaultInterval = 6 * time.Hour
	memoryIndexReconcileMaxAttempts     = 5
	memoryIndexReconcileBatchSize       = 200
)

type MemoryFactIndexReconciler struct {
	interval   time.Duration
	maintainer biz.MemoryFactIndexMaintainer
	indexSync  biz.MemoryFactIndexSyncer
	lg         loggateway.Logger
}

func NewMemoryFactIndexReconciler(interval time.Duration, maintainer biz.MemoryFactIndexMaintainer, indexSync biz.MemoryFactIndexSyncer, lg loggateway.Logger) *MemoryFactIndexReconciler {
	if interval <= 0 {
		interval = memoryIndexReconcileDefaultInterval
	}
	return &MemoryFactIndexReconciler{
		interval:   interval,
		maintainer: maintainer,
		indexSync:  indexSync,
		lg:         lg,
	}
}

func (w *MemoryFactIndexReconciler) Start(ctx context.Context) {
	if w == nil || w.maintainer == nil || w.indexSync == nil {
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
		rows, err := w.maintainer.ListStaleIndexFacts(ctx, memoryIndexReconcileMaxAttempts, memoryIndexReconcileBatchSize)
		if err != nil {
			w.lg.Warn("list stale facts failed", loggateway.Err(err))
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
						if disableErr := w.maintainer.MarkFactIndexDisabled(ctx, factID); disableErr == nil {
							disabled++
							w.lg.Warn("fact index permanently disabled after max attempts", loggateway.Str("fact_id", factID), loggateway.Int("attempts", row.IndexAttempts))
						}
					}
				}
				continue
			}
			synced++
		}
		w.lg.Info("memory index reconcile completed", loggateway.Int("synced", synced), loggateway.Int("failed", failed), loggateway.Int("disabled", disabled), loggateway.Int("total", len(rows)))
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
