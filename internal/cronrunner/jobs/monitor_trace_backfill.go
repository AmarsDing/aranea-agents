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
	traceRepo        biz.MonitorTraceRepo
	runnerCompletion biz.MonitorRunnerCompletionRepo
	usageRepo        biz.MonitorTraceUsageRepo
	interval         time.Duration
	watermark        string
	lg               loggateway.Logger
}

func defaultBackfillInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MONITOR_TRACE_BACKFILL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 6 * time.Hour
}

// staleRunningTraceTTL bounds how long a trace may stay in "running" before
// the sweeper marks it "interrupted" (process crashed before completion).
const staleRunningTraceTTL = 30 * time.Minute

func NewMonitorTraceBackfillWorker(traceRepo biz.MonitorTraceRepo, runnerCompletion biz.MonitorRunnerCompletionRepo, usageRepo biz.MonitorTraceUsageRepo, lg loggateway.Logger) *MonitorTraceBackfillWorker {
	return &MonitorTraceBackfillWorker{
		traceRepo:        traceRepo,
		runnerCompletion: runnerCompletion,
		usageRepo:        usageRepo,
		interval:         defaultBackfillInterval(),
		lg:               lg,
	}
}

func (w *MonitorTraceBackfillWorker) Start(ctx context.Context) {
	if w == nil || w.traceRepo == nil {
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
	if err := w.traceRepo.EnsureTraceSchema(ctx); err != nil {
		w.lg.Warn("backfill: EnsureTraceSchema failed", loggateway.Err(err))
		return
	}
	w.sweepStaleRunning(ctx)
	since := 30 * 24 * time.Hour
	if w.watermark != "" {
		if wm, err := time.Parse(time.RFC3339, w.watermark); err == nil {
			elapsed := time.Since(wm)
			if elapsed < since {
				since = elapsed + time.Hour
			}
		}
	}
	rows, err := w.runnerCompletion.ListRecentRunnerCompletions(ctx, since, 1000)
	if err != nil {
		w.lg.Warn("backfill: ListRecentRunnerCompletions failed", loggateway.Err(err))
		return
	}
	inserted := 0
	updated := 0
	var latestCreatedAt string
	for _, row := range rows {
		if row.TraceID == "" {
			continue
		}
		agg := w.aggregateUsage(ctx, row.TraceID)
		tw := monitor.TraceWrite{
			TraceID:    row.TraceID,
			SessionID:  row.SessionID,
			RunID:      row.RunID,
			AgentID:    row.AgentID,
			Provider:   agg.Provider,
			Model:      agg.Model,
			Name:       "runner.completion",
			Status:     row.Status,
			DurationMs: row.DurationMs,
		}
		if err := w.traceRepo.InsertMonitorTrace(ctx, tw); err != nil {
			w.lg.Warn("backfill: InsertMonitorTrace failed",
				loggateway.Str("trace_id", tw.TraceID),
				loggateway.Err(err))
		} else {
			inserted++
		}
		// Always close/update existing running rows to terminal status (INSERT OR IGNORE
		// alone cannot advance rows already inserted by TraceProjector).
		errCount := 0
		if tw.Status == "error" {
			errCount = 1
		}
		if err := w.traceRepo.UpdateMonitorTraceCompletion(ctx, tw.TraceID, monitor.TraceCompletion{
			Status:       tw.Status,
			DurationMs:   tw.DurationMs,
			SpanCount:    0,
			ErrorCount:   errCount,
			TotalTokens:  agg.TotalTokens,
			TotalCostUsd: agg.TotalCostUsd,
			Provider:     agg.Provider,
			Model:        agg.Model,
		}); err != nil {
			w.lg.Warn("backfill: UpdateMonitorTraceCompletion failed",
				loggateway.Str("trace_id", tw.TraceID),
				loggateway.Err(err))
			continue
		}
		updated++
	}
	if len(rows) > 0 {
		// ListRecentRunnerCompletions returns rows ordered by created_at DESC,
		// so rows[0] is the newest record. Using rows[len-1] would pick the
		// oldest row and cause the watermark to regress on each run.
		lastRow := rows[0]
		if lastRow.CreatedAt != "" {
			latestCreatedAt = lastRow.CreatedAt
		} else {
			latestCreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		w.watermark = latestCreatedAt
	}
	if inserted > 0 || updated > 0 {
		w.lg.Info("monitor trace backfill completed",
			loggateway.Int("inserted", inserted),
			loggateway.Int("updated", updated),
			loggateway.Str("since", since.String()))
	}
}

// aggregateUsage sums tokens/cost and resolves provider/model from usage
// events for the trace. Failures degrade to zero values (best-effort backfill).
func (w *MonitorTraceBackfillWorker) aggregateUsage(ctx context.Context, traceID string) monitor.UsageAggregate {
	if w.usageRepo == nil {
		return monitor.UsageAggregate{}
	}
	agg, err := w.usageRepo.AggregateUsageByTrace(ctx, traceID)
	if err != nil {
		w.lg.Warn("backfill: AggregateUsageByTrace failed",
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err))
		return monitor.UsageAggregate{}
	}
	return agg
}

// sweepStaleRunning marks traces stuck in "running" beyond the TTL as
// "interrupted" so the UI does not show phantom in-flight runs.
func (w *MonitorTraceBackfillWorker) sweepStaleRunning(ctx context.Context) {
	cutoff := time.Now().Add(-staleRunningTraceTTL)
	n, err := w.traceRepo.InterruptStaleTraces(ctx, cutoff)
	if err != nil {
		w.lg.Warn("backfill: InterruptStaleTraces failed", loggateway.Err(err))
		return
	}
	if n > 0 {
		w.lg.Info("swept stale running traces", loggateway.Int("interrupted", int(n)))
	}
}
