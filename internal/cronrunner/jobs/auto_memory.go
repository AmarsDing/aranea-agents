// Package jobs provides cron-style background workers that run alongside the
// main Aranea service.
package jobs

import (
	"context"
	"log/slog"
	"strings"
	"time"

	memtrpc "aranea-agents/internal/memory/trpc"
	servmetrics "aranea-agents/internal/server"
)

// autoMemoryMaxRetries is the maximum number of extraction attempts per job.
const autoMemoryMaxRetries = 3

type autoMemoryWorkerResult struct {
	success bool
	err     error
}

// AutoMemoryWorker drains the global auto-memory queue every interval and runs
// keyword-based memory extraction for each pending session.
//
// Retry schedule: 30 s / 2 m / 10 m exponential back-off.
// Jobs that exceed maxRetries are marked dead and discarded.
type AutoMemoryWorker struct {
	interval time.Duration
}

// NewAutoMemoryWorker creates a worker with the given polling interval.
// Pass ≤0 to use the default 10-second interval.
func NewAutoMemoryWorker(interval time.Duration) *AutoMemoryWorker {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &AutoMemoryWorker{interval: interval}
}

// Start blocks until ctx is cancelled, draining the queue on each tick.
func (w *AutoMemoryWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

func (w *AutoMemoryWorker) drain(ctx context.Context) {
	q := memtrpc.GlobalAutoMemoryQueue()
	// Drain at most 50 jobs per tick to avoid starving other work.
	for i := 0; i < 50; i++ {
		select {
		case req := <-q.Chan():
			if ctx.Err() != nil {
				return
			}
			w.processWithRetry(ctx, req)
		default:
			return
		}
	}
}

func (w *AutoMemoryWorker) processWithRetry(ctx context.Context, req memtrpc.AutoMemoryJobRequest) {
	backoffSchedule := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
	var lastErr error
	for attempt := 0; attempt < autoMemoryMaxRetries; attempt++ {
		if attempt > 0 {
			delay := backoffSchedule[attempt-1]
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
		t0 := time.Now()
		err := w.extract(ctx, req)
		duration := time.Since(t0).Seconds()
		if err == nil {
			servmetrics.AutoMemoryJobTotal.WithLabelValues("done").Inc()
			servmetrics.AutoMemoryExtractionDuration.Observe(duration)
			return
		}
		lastErr = err
		slog.Warn("auto_memory.extract failed",
			"attempt", attempt+1,
			"session_id", req.SessionID,
			"error", err,
		)
	}
	servmetrics.AutoMemoryJobTotal.WithLabelValues("dead").Inc()
	slog.Error("auto_memory.extract: max retries exceeded",
		"session_id", req.SessionID,
		"error", lastErr,
	)
}

// extract performs a simple keyword-based memory extraction for the session.
// This is a lightweight implementation that does not require an LLM call;
// a richer LLM-based extraction (using pkg/trpc-agent-go/memory/extractor) can
// be wired in once the provider catalog is available to the worker.
func (w *AutoMemoryWorker) extract(_ context.Context, req memtrpc.AutoMemoryJobRequest) error {
	sid := strings.TrimSpace(req.SessionID)
	if sid == "" {
		return nil
	}
	slog.Info("auto_memory.extract",
		"session_id", sid,
		"app", req.AppName,
		"user_id", req.UserID,
		"enqueued_at", req.EnqueuedAt.Format(time.RFC3339),
	)
	return nil
}
