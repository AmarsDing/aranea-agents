package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

type MonitorTraceBackfillWorker struct {
	repo     biz.MonitorRepo
	interval time.Duration
	log      *log.Helper
}

func defaultBackfillInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MONITOR_TRACE_BACKFILL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 6 * time.Hour
}

func NewMonitorTraceBackfillWorker(repo biz.MonitorRepo, logger log.Logger) *MonitorTraceBackfillWorker {
	return &MonitorTraceBackfillWorker{
		repo:     repo,
		interval: defaultBackfillInterval(),
		log:      log.NewHelper(logger),
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
	rows, err := w.repo.ListRecentRunnerCompletions(ctx, 30*24*time.Hour, 1000)
	if err != nil {
		event.SysLogWarn("system.monitor.trace_backfill_query_fail", "backfill: ListRecentRunnerCompletions failed", event.P("error", err.Error()))
		return
	}
	inserted := 0
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
	if inserted > 0 && w.log != nil {
		w.log.Debugf("monitor trace backfill: inserted %d traces", inserted)
	}
}
