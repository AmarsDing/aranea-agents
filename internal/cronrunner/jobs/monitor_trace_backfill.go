package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

type MonitorTraceBackfillWorker struct {
	repo      biz.MonitorRepo
	interval  time.Duration
	watermark string
}

func defaultBackfillInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MONITOR_TRACE_BACKFILL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 6 * time.Hour
}

func NewMonitorTraceBackfillWorker(repo biz.MonitorRepo) *MonitorTraceBackfillWorker {
	return &MonitorTraceBackfillWorker{
		repo:     repo,
		interval: defaultBackfillInterval(),
	}
}

func (w *MonitorTraceBackfillWorker) Start(ctx context.Context) {
	if w == nil || w.repo == nil {
		return
	}
	safego.Go(ctx, "monitor.trace-backfill", func() {
		w.runOnce(ctx)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.runOnce(ctx)
			}
		}
	})
}

func (w *MonitorTraceBackfillWorker) runOnce(ctx context.Context) {
	if err := w.repo.EnsureTraceSchema(ctx); err != nil {
		event.SysLogWarn("system.monitor.trace_backfill_schema_fail", "backfill: EnsureTraceSchema failed", event.P("error", err.Error()))
		return
	}
	since := 30 * 24 * time.Hour
	if w.watermark != "" {
		if wm, err := time.Parse(time.RFC3339, w.watermark); err == nil {
			elapsed := time.Since(wm)
			if elapsed < since {
				since = elapsed + time.Hour
			}
		}
	}
	rows, err := w.repo.ListRecentRunnerCompletions(ctx, since, 1000)
	if err != nil {
		event.SysLogWarn("system.monitor.trace_backfill_query_fail", "backfill: ListRecentRunnerCompletions failed", event.P("error", err.Error()))
		return
	}
	inserted := 0
	var latestCreatedAt string
	for _, row := range rows {
		tw := monitor.TraceWrite{
			TraceID:   row.TraceID,
			SessionID: row.SessionID,
			RunID:     row.RunID,
			AgentID:   row.AgentID,
			Name:      "runner.completion",
			Status:    row.Status,
		}
		if tw.TraceID == "" {
			continue
		}
		if err := w.repo.InsertMonitorTrace(ctx, tw); err != nil {
			continue
		}
		inserted++
	}
	if len(rows) > 0 {
		lastRow := rows[len(rows)-1]
		if lastRow.CreatedAt != "" {
			latestCreatedAt = lastRow.CreatedAt
		} else {
			latestCreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		w.watermark = latestCreatedAt
	}
	if inserted > 0 {
		event.SysLogInfo("system.monitor.trace_backfill_done", "monitor trace backfill completed",
			event.P("inserted", fmt.Sprint(inserted)),
			event.P("since", since.String()))
	}
}
