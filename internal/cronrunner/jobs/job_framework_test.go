package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// TestJobRunner_Success verifies that a successful job returns nil and
// increments the "done" counter.
func TestJobRunner_Success(t *testing.T) {
	runner := NewJobRunner(loggateway.NewNoop())

	var callCount int32
	err := runner.Run(context.Background(), JobConfig{
		JobID:      "test_success",
		MaxRetries: 0, // no retry
	}, func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
}

// TestJobRunner_RetryThenSucceed verifies that the runner retries on error
// and succeeds when a later attempt returns nil.
func TestJobRunner_RetryThenSucceed(t *testing.T) {
	runner := NewJobRunner(loggateway.NewNoop())

	var callCount int32
	err := runner.Run(context.Background(), JobConfig{
		JobID:      "test_retry_success",
		MaxRetries: 3,
		Backoff:    []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond},
	}, func(ctx context.Context) error {
		n := atomic.AddInt32(&callCount, 1)
		if n < 3 {
			return errors.New("transient failure")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error after retries, got %v", err)
	}
	if got := atomic.LoadInt32(&callCount); got != 3 {
		t.Fatalf("expected 3 calls (2 retries), got %d", got)
	}
}

// TestJobRunner_ExhaustedRetries verifies that the runner returns the last
// error and counts the job as "dead" when all retries are exhausted.
func TestJobRunner_ExhaustedRetries(t *testing.T) {
	runner := NewJobRunner(loggateway.NewNoop())

	var callCount int32
	finalErr := errors.New("permanent failure")
	err := runner.Run(context.Background(), JobConfig{
		JobID:      "test_exhausted",
		MaxRetries: 2,
		Backoff:    []time.Duration{1 * time.Millisecond, 2 * time.Millisecond},
	}, func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return finalErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, finalErr) {
		t.Fatalf("expected error %v, got %v", finalErr, err)
	}
	// 1 initial + 2 retries = 3 attempts
	if got := atomic.LoadInt32(&callCount); got != 3 {
		t.Fatalf("expected 3 calls, got %d", got)
	}
}

// TestJobRunner_PanicRecovery verifies that panics are recovered and treated
// as errors, allowing retry logic to continue.
func TestJobRunner_PanicRecovery(t *testing.T) {
	runner := NewJobRunner(loggateway.NewNoop())

	var callCount int32
	err := runner.Run(context.Background(), JobConfig{
		JobID:      "test_panic",
		MaxRetries: 2,
		Backoff:    []time.Duration{1 * time.Millisecond, 2 * time.Millisecond},
	}, func(ctx context.Context) error {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			panic("boom")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error after panic recovery + retry, got %v", err)
	}
	if got := atomic.LoadInt32(&callCount); got != 2 {
		t.Fatalf("expected 2 calls (1 panic + 1 success), got %d", got)
	}
}

// TestJobRunner_ContextCancellation verifies that the runner respects context
// cancellation during backoff.
func TestJobRunner_ContextCancellation(t *testing.T) {
	runner := NewJobRunner(loggateway.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callCount int32
	// Cancel the context after the first failure (during backoff).
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := runner.Run(ctx, JobConfig{
		JobID:      "test_cancel",
		MaxRetries: 3,
		Backoff:    []time.Duration{500 * time.Millisecond, 1 * time.Second, 2 * time.Second},
	}, func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return errors.New("fail")
	})

	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestJobRunner_DefaultMaxRetries verifies that a negative MaxRetries falls
// back to DefaultJobMaxRetries.
func TestJobRunner_DefaultMaxRetries(t *testing.T) {
	runner := NewJobRunner(loggateway.NewNoop())

	var callCount int32
	err := runner.Run(context.Background(), JobConfig{
		JobID:      "test_default_retries",
		MaxRetries: -1, // should default to DefaultJobMaxRetries (3)
		Backoff:    []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond},
	}, func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return errors.New("fail")
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// 1 initial + 3 retries = 4 attempts
	expected := int32(1 + DefaultJobMaxRetries)
	if got := atomic.LoadInt32(&callCount); got != expected {
		t.Fatalf("expected %d calls, got %d", expected, got)
	}
}

// TestJobRunner_ZeroMaxRetries verifies that MaxRetries=0 disables retry.
func TestJobRunner_ZeroMaxRetries(t *testing.T) {
	runner := NewJobRunner(loggateway.NewNoop())

	var callCount int32
	_ = runner.Run(context.Background(), JobConfig{
		JobID:      "test_no_retry",
		MaxRetries: 0,
	}, func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return errors.New("fail")
	})

	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", got)
	}
}

// TestJobRunner_NilLogger verifies that the runner works with a nil logger.
func TestJobRunner_NilLogger(t *testing.T) {
	runner := NewJobRunner(nil)

	err := runner.Run(context.Background(), JobConfig{
		JobID:      "test_nil_logger",
		MaxRetries: 0,
	}, func(ctx context.Context) error {
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
