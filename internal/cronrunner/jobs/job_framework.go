// Package jobs provides cron-style background workers that run alongside the
// main Aranea service.
package jobs

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// DefaultJobRetryBackoff is the 3-step backoff schedule: 30s, 2m, 10m.
// Extracted from cronrunner/runner.go and auto_memory.go processWithRetry.
var DefaultJobRetryBackoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

// DefaultJobMaxRetries is the default number of retries after the first attempt.
const DefaultJobMaxRetries = 3

// JobFunc is the function executed by a JobRunner. It returns an error if the
// job fails; the runner will retry according to the backoff schedule.
type JobFunc func(ctx context.Context) error

// DeadLetterEntry captures the metadata of a job that exhausted all retries
// and is being sent to the dead-letter sink.
type DeadLetterEntry struct {
	JobID      string
	Err        error
	Attempts   int
	OccurredAt time.Time
}

// DeadLetterWriter is an optional sink for jobs that exhausted all retries.
// When set on a JobRunner, the runner writes a DeadLetterEntry after the final
// retry fails so the dead-letter replayer can re-enqueue the job later.
type DeadLetterWriter interface {
	WriteDeadLetter(entry DeadLetterEntry) error
}

// JobConfig describes a job's execution parameters.
type JobConfig struct {
	// JobID is the unique identifier for the job (used in metrics and logs).
	JobID string
	// MaxRetries is the number of retries after the first attempt.
	// 0 disables retry; negative values default to DefaultJobMaxRetries.
	MaxRetries int
	// Backoff is the retry backoff schedule. If nil, DefaultJobRetryBackoff is used.
	Backoff []time.Duration
}

// Package-level metrics for background jobs (following the pattern in
// cronrunner/runner.go). Names are prefixed with `aranea_background_job_` to
// avoid collision with the existing `aranea_cron_job_*` metrics.
var (
	jobRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_background_job_runs_total",
		Help: "Number of background job executions by job_id and status.",
	}, []string{"job_id", "status"})

	jobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_background_job_duration_seconds",
		Help:    "Duration of background job executions.",
		Buckets: []float64{0.5, 1, 5, 15, 30, 60, 120, 300, 600},
	}, []string{"job_id"})

	jobDeadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_background_job_dead_total",
		Help: "Number of background jobs that reached the dead-letter state.",
	}, []string{"job_id"})
)

// JobRunner executes JobFunc with retry, panic recovery, and metrics.
//
// This is the unified Job framework extracted from:
//   - cronrunner/runner.go dispatchWithRetry + dispatchSafe (retry + panic recovery)
//   - auto_memory.go processWithRetry (30s/2m/10m backoff + dead letter)
//
// Usage:
//
//	runner := jobs.NewJobRunner(lg)
//	err := runner.Run(ctx, jobs.JobConfig{
//	    JobID:      "memory_ebbinghaus_decay",
//	    MaxRetries: jobs.DefaultJobMaxRetries,
//	}, func(ctx context.Context) error {
//	    return scanAndCompute(ctx)
//	})
type JobRunner struct {
	lg         loggateway.Logger
	breaker    *CircuitBreaker  // optional, nil disables circuit breaking
	deadLetter DeadLetterWriter // optional, nil disables dead-letter writes
}

// NewJobRunner creates a JobRunner. When lg is nil, a no-op logger is used.
func NewJobRunner(lg loggateway.Logger) *JobRunner {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &JobRunner{lg: lg}
}

// WithCircuitBreaker attaches a circuit breaker to the runner. When set, the
// runner checks Allow() before each attempt and records success/failure after.
// Returns the receiver for chaining.
func (r *JobRunner) WithCircuitBreaker(cb *CircuitBreaker) *JobRunner {
	if r != nil {
		r.breaker = cb
	}
	return r
}

// WithDeadLetter attaches a dead-letter writer to the runner. When set, the
// runner writes a DeadLetterEntry after all retries are exhausted.
// Returns the receiver for chaining.
func (r *JobRunner) WithDeadLetter(w DeadLetterWriter) *JobRunner {
	if r != nil {
		r.deadLetter = w
	}
	return r
}

// Run executes the job with retry and panic recovery.
//
// The job is retried up to MaxRetries times with exponential backoff.
// Panics inside the job are recovered and treated as errors.
// When all retries are exhausted, the job is counted as "dead" in metrics
// and, if a DeadLetterWriter is attached, a DeadLetterEntry is written.
//
// When a CircuitBreaker is attached and the circuit is Open, Run returns
// immediately with a sentinel error without executing the job.
//
// Returns the final error (nil on success).
func (r *JobRunner) Run(ctx context.Context, cfg JobConfig, fn JobFunc) error {
	// Circuit breaker: reject early when the circuit is open.
	if r != nil && r.breaker != nil && !r.breaker.Allow() {
		jobRunsTotal.WithLabelValues(cfg.JobID, "rejected").Inc()
		return fmt.Errorf("circuit breaker open for job %q", cfg.JobID)
	}

	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = DefaultJobMaxRetries
	}
	backoff := cfg.Backoff
	if backoff == nil {
		backoff = DefaultJobRetryBackoff
	}

	attempts := 1
	if maxRetries > 0 {
		if maxRetries < len(backoff) {
			backoff = backoff[:maxRetries]
		}
		attempts = len(backoff) + 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := backoff[attempt-1]
			r.lg.Warn("background job retry",
				loggateway.Str("job_id", cfg.JobID),
				loggateway.Int("attempt", attempt+1),
				loggateway.Str("delay", delay.String()))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		t0 := time.Now()
		err := r.runSafe(ctx, cfg, fn)
		jobDuration.WithLabelValues(cfg.JobID).Observe(time.Since(t0).Seconds())

		if err == nil {
			jobRunsTotal.WithLabelValues(cfg.JobID, "done").Inc()
			if r != nil && r.breaker != nil {
				r.breaker.RecordSuccess()
			}
			return nil
		}
		lastErr = err
		if r != nil && r.breaker != nil {
			r.breaker.RecordFailure()
		}
		r.lg.Warn("background job failed",
			loggateway.Str("job_id", cfg.JobID),
			loggateway.Int("attempt", attempt+1),
			loggateway.Err(err))
	}

	jobRunsTotal.WithLabelValues(cfg.JobID, "dead").Inc()
	jobDeadTotal.WithLabelValues(cfg.JobID).Inc()
	r.lg.Error("background job exhausted retries",
		loggateway.Str("job_id", cfg.JobID),
		loggateway.Err(lastErr))

	// Dead-letter: persist the failed job for later replay.
	if r != nil && r.deadLetter != nil {
		entry := DeadLetterEntry{
			JobID:      cfg.JobID,
			Err:        lastErr,
			Attempts:   attempts,
			OccurredAt: time.Now(),
		}
		if dlErr := r.deadLetter.WriteDeadLetter(entry); dlErr != nil {
			r.lg.Warn("background job dead-letter write failed",
				loggateway.Str("job_id", cfg.JobID),
				loggateway.Err(dlErr))
		}
	}
	return lastErr
}

// runSafe wraps fn with panic recovery. Panics are converted to errors so the
// retry loop can handle them uniformly.
func (r *JobRunner) runSafe(ctx context.Context, cfg JobConfig, fn JobFunc) (retErr error) {
	defer func() {
		if rec := recover(); rec != nil {
			retErr = fmt.Errorf("job panic: %v", rec)
			r.lg.Error("background job panic",
				loggateway.Str("job_id", cfg.JobID),
				loggateway.Any("panic", rec))
		}
	}()
	return fn(ctx)
}

// DeadLetterSinkAdapter adapts a biz.MemoryDeadLetterSink to the
// DeadLetterWriter interface. This allows background jobs (e.g. the Sleep-time
// worker) to reuse the existing memory_job_deadletter table for persisting
// exhausted jobs without introducing a new persistence layer.
//
// The adapter stores the JobID in the AppName field (prefixed with
// "background:") and the error details in last_error. The drop_reason is set
// to "retry_exhausted" so dead-letter admin queries can distinguish
// background-job failures from AutoMemory enqueue failures.
type DeadLetterSinkAdapter struct {
	sink biz.MemoryDeadLetterSink
}

// NewDeadLetterSinkAdapter creates a DeadLetterSinkAdapter. When sink is nil,
// the adapter is a no-op (WriteDeadLetter returns nil without writing).
func NewDeadLetterSinkAdapter(sink biz.MemoryDeadLetterSink) *DeadLetterSinkAdapter {
	return &DeadLetterSinkAdapter{sink: sink}
}

// WriteDeadLetter implements DeadLetterWriter by delegating to the wrapped
// MemoryDeadLetterSink. The sink's WriteMemoryDeadLetter is best-effort (it
// logs internally on failure), so this method always returns nil.
func (a *DeadLetterSinkAdapter) WriteDeadLetter(entry DeadLetterEntry) error {
	if a == nil || a.sink == nil {
		return nil
	}
	errMsg := ""
	if entry.Err != nil {
		errMsg = entry.Err.Error()
	}
	a.sink.WriteMemoryDeadLetter(
		biz.MemoryDeadLetterRequest{
			AppName:  "background:" + entry.JobID,
			Priority: biz.MemoryJobPriorityLow,
		},
		biz.MemoryDeadLetterReasonRetryExhausted,
		fmt.Sprintf("job=%s attempts=%d err=%s", entry.JobID, entry.Attempts, errMsg),
	)
	return nil
}

// Compile-time interface check.
var _ DeadLetterWriter = (*DeadLetterSinkAdapter)(nil)
