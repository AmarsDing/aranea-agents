package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"aranea-agents/internal/biz/backgroundjob"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// backgroundJobWorkerPollInterval is the default interval between claim
// attempts. Kept conservative (5s) to avoid hammering SQLite while still
// providing responsive job dispatch for real-time-priority jobs.
const backgroundJobWorkerPollInterval = 5 * time.Second

// BackgroundJobWorker polls the backgroundjob.Repo for queued jobs, claims
// them via TryClaim, dispatches to the registered Runner for the job's Kind,
// and marks the job succeeded/failed based on the Runner's return value.
//
// It is the runtime component of the Unified BackgroundJob subsystem
// (M56 BLO-5). Without this worker, the background_jobs table is inert —
// jobs are enqueued but never processed.
//
// Design notes:
//   - The worker uses a single goroutine per instance. Concurrency is
//     achieved by running multiple worker instances with different workerIDs,
//     each claiming distinct jobs via the atomic TryClaim path.
//   - Processing uses context.Background() (not the parent ctx) so that
//     HTTP request cancellation or graceful shutdown signals do not
//     interrupt in-flight job execution mid-way; the worker exits only
//     between jobs when ctx.Done() fires.
//   - If no Runner is registered for a claimed job's Kind, the job is
//     marked failed with a descriptive error. This prevents unprocessable
//     jobs from blocking the queue indefinitely.
type BackgroundJobWorker struct {
	repo        backgroundjob.Repo
	registry    backgroundjob.Registry
	lg          loggateway.Logger
	pollInterval time.Duration
	workerID    string
}

// NewBackgroundJobWorker constructs a BackgroundJobWorker. Returns nil if
// any required dependency is nil (defensive construction per red-line #26).
//
// The workerID is derived from the hostname + PID to uniquely identify this
// worker instance in the claimed_by column. If hostname resolution fails,
// a fallback ID is used.
func NewBackgroundJobWorker(repo backgroundjob.Repo, registry backgroundjob.Registry, lg loggateway.Logger) *BackgroundJobWorker {
	if repo == nil || registry == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &BackgroundJobWorker{
		repo:         repo,
		registry:     registry,
		lg:           lg.With(loggateway.Domain("backgroundjob")),
		pollInterval: backgroundJobWorkerPollInterval,
		workerID:     resolveWorkerID(),
	}
}

// resolveWorkerID returns a unique identifier for this worker process.
// Format: hostname:pid. Falls back to "worker:<pid>" if hostname is empty.
func resolveWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "worker"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

// Start launches the worker in a background goroutine via safego.Go
// (red-line #13). The goroutine polls the repo on pollInterval until ctx
// is cancelled (red-line #23 goroutine exit path).
//
// Start returns immediately; the worker runs asynchronously. Callers do
// not need to wait on it.
func (w *BackgroundJobWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	kinds := w.registry.Kinds()
	if len(kinds) == 0 {
		// No runners registered — nothing to do. Log once and exit without
		// starting a polling loop (avoids wasted CPU on a no-op worker).
		w.lg.Info("backgroundjob worker not started: no runners registered",
			loggateway.StepID("backgroundjob.worker"),
		)
		return
	}
	w.lg.Info("backgroundjob worker starting",
		loggateway.StepID("backgroundjob.worker"),
		loggateway.Str("worker_id", w.workerID),
		loggateway.Str("kinds", fmt.Sprintf("%v", kinds)),
		loggateway.Str("poll_interval", w.pollInterval.String()),
	)
	safego.Go(ctx, "backgroundjob-worker", func() {
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				w.lg.Info("backgroundjob worker stopped",
					loggateway.StepID("backgroundjob.worker"),
					loggateway.Str("worker_id", w.workerID),
				)
				return
			case <-ticker.C:
				w.processOnce(context.Background())
			}
		}
	})
}

// processOnce claims and processes a single job. It returns silently when
// no job is available. Errors are logged but do not stop the worker —
// transient DB errors should not kill the polling loop.
func (w *BackgroundJobWorker) processOnce(ctx context.Context) {
	kinds := w.registry.Kinds()
	if len(kinds) == 0 {
		return
	}
	job, err := w.repo.TryClaim(ctx, w.workerID, kinds)
	if err != nil {
		w.lg.Warn("claim failed",
			loggateway.StepID("backgroundjob.worker"),
			loggateway.Str("worker_id", w.workerID),
			loggateway.Err(err),
		)
		return
	}
	if job == nil {
		// No claimable job — normal idle state, no log to avoid noise.
		return
	}

	runner := w.registry.Lookup(job.Kind)
	if runner == nil {
		// No runner for this kind — mark failed so it doesn't block the queue.
		// This is a programming error (worker claimed a kind it can't process),
		// but we handle it gracefully.
		errMsg := fmt.Sprintf("no runner registered for kind %q", job.Kind)
		w.lg.Error("no runner for claimed job",
			loggateway.StepID("backgroundjob.worker"),
			loggateway.Str("job_id", job.ID),
			loggateway.Str("kind", job.Kind),
			loggateway.Str("worker_id", w.workerID),
		)
		if ferr := w.repo.MarkFailed(ctx, job.ID, errMsg); ferr != nil {
			w.lg.Warn("mark failed after missing runner",
				loggateway.StepID("backgroundjob.worker"),
				loggateway.Str("job_id", job.ID),
				loggateway.Err(ferr),
			)
		}
		return
	}

	w.lg.Info("processing job",
		loggateway.StepID("backgroundjob.worker"),
		loggateway.Str("job_id", job.ID),
		loggateway.Str("kind", job.Kind),
		loggateway.Str("owner_type", string(job.OwnerType)),
		loggateway.Str("owner_id", job.OwnerID),
		loggateway.Int("attempt", job.Attempts),
		loggateway.Str("worker_id", w.workerID),
	)

	if err := runner.Run(ctx, *job); err != nil {
		w.lg.Warn("job failed",
			loggateway.StepID("backgroundjob.worker"),
			loggateway.Str("job_id", job.ID),
			loggateway.Str("kind", job.Kind),
			loggateway.Int("attempt", job.Attempts),
			loggateway.Int("max_attempts", job.MaxAttempts),
			loggateway.Err(err),
		)
		if ferr := w.repo.MarkFailed(ctx, job.ID, err.Error()); ferr != nil {
			w.lg.Warn("mark failed after runner error",
				loggateway.StepID("backgroundjob.worker"),
				loggateway.Str("job_id", job.ID),
				loggateway.Err(ferr),
			)
		}
		return
	}

	if err := w.repo.MarkSucceeded(ctx, job.ID); err != nil {
		w.lg.Warn("mark succeeded failed",
			loggateway.StepID("backgroundjob.worker"),
			loggateway.Str("job_id", job.ID),
			loggateway.Err(err),
		)
		return
	}
	w.lg.Info("job succeeded",
		loggateway.StepID("backgroundjob.worker"),
		loggateway.Str("job_id", job.ID),
		loggateway.Str("kind", job.Kind),
		loggateway.Int("attempt", job.Attempts),
	)
}
