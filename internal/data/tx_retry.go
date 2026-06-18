package data

import (
	"context"
	"errors"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

// ExecInTxWithRetry wraps ExecInTx with automatic retry on transient DB
// errors. It implements the No-Timeout principle (T2.1): DB transient
// failures (deadlock, serialization failure, tx timeout) are recovered
// via exponential backoff retry, so long-running tasks don't fail on
// transient DB contention.
//
// Retry policy:
//   - Retries on: apierror.CodeInternal (unknown DB errors), Postgres
//     deadlock_detected (40P01), serialization_failure (40001),
//     context.DeadlineExceeded (tx timeout, not caller cancel).
//   - Does NOT retry on: context.Canceled (caller cancelled),
//     apierror.CodeConflict/BadRequest/NotFound (business errors that
//     retry won't fix).
//   - Backoff: exponential, 1s → 2s → 4s, max 3 retries (CS-B15).
//
// The fn callback MUST be idempotent — it may be invoked multiple times.
// Side effects (event publishing, WS push) must be deferred until after
// the transaction commits successfully, otherwise retries may duplicate
// them.
//
// Caller's ctx cancellation is respected: if the caller cancels during
// the backoff sleep, the function returns ctx.Err() immediately without
// retrying.
func (d *Data) ExecInTxWithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	if d == nil {
		return apierror.Internal(apierror.DomainData, "data not initialized")
	}
	const maxRetries = 3
	const baseDelay = 1 * time.Second
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Don't attempt if the caller already cancelled.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := d.ExecInTx(ctx, fn)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableDBError(err) {
			return err
		}
		if attempt == maxRetries {
			break
		}
		delay := baseDelay * time.Duration(1<<attempt) // 1s, 2s, 4s
		if d.lg != nil {
			d.lg.Warn("db transaction retrying after transient error",
				loggateway.StepID("data.tx_retry"),
				loggateway.Int("attempt", attempt+1),
				loggateway.Int("max_retries", maxRetries),
				loggateway.Int64("delay_ms", int64(delay/time.Millisecond)),
				loggateway.Err(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// isRetryableDBError reports whether err represents a transient DB error
// that is safe to retry. Non-retryable errors include business errors
// (conflict, bad request, not found) and caller cancellation.
func isRetryableDBError(err error) bool {
	if err == nil {
		return false
	}
	// Caller cancellation — never retry.
	if errors.Is(err, context.Canceled) {
		return false
	}
	// apierror classification: only CodeInternal is retryable (unknown
	// DB error). Business errors (Conflict/BadRequest/NotFound) are not.
	if ae, ok := apierror.From(err); ok {
		switch ae.Code {
		case apierror.CodeInternal:
			return true
		case apierror.CodeConflict, apierror.CodeBadRequest, apierror.CodeNotFound:
			return false
		}
	}
	// Postgres deadlock / serialization failure — retry.
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		switch pgErr.Code.Name() {
		case "deadlock_detected", "serialization_failure":
			return true
		}
	}
	// context.DeadlineExceeded — retry (tx timeout, not caller cancel).
	// ExecInTx uses a detached context with txTimeout for the tx itself;
	// a deadline exceeded here means the tx hit its safety-net timeout,
	// which is transient (contention will clear).
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}
