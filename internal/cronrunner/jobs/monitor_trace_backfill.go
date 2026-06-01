package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

type MonitorTraceBackfillWorker struct {
	repo      biz.MonitorRepo
	interval  time.Duration
	watermark string
	lg        loggateway.Logger
}

func defaultBackfillInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MONITOR_TRACE_BACKFILL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 6 * time.Hour
}

func NewMonitorTraceBackfillWorker(repo biz.MonitorRepo, lg loggateway.Logger) *MonitorTraceBackfillWorker {
	return &MonitorTraceBackfillWorker{
		repo:     repo,
		interval: defaultBackfillInterval(),
		lg:       lg,
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
		w.lg.Warn("backfill: EnsureTraceSchema failed", loggateway.Err(err))
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
		w.lg.Warn("backfill: ListRecentRunnerCompletions failed", loggateway.Err(err))
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
		w.lg.Info("monitor trace backfill completed",
			loggateway.Int("inserted", inserted),
			loggateway.Str("since", since.String()))
	}
}
