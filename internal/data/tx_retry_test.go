package data

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

func newTestDataForRetry() *Data {
	return &Data{lg: loggateway.NewNoop()}
}

func TestExecInTxWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	d := newTestDataForRetry()
	var calls int32
	err := d.ExecInTxWithRetry(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestExecInTxWithRetry_RetriesOnDeadlock(t *testing.T) {
	d := newTestDataForRetry()
	// Simulate a Postgres deadlock inside fn. ExecInTx calls fn and returns
	// its error directly after rollback, so a pq.Error from fn correctly
	// simulates a DB deadlock that isRetryableDBError detects via errors.As.
	var calls int32
	err := d.ExecInTxWithRetry(context.Background(), func(ctx context.Context) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return &pq.Error{Code: "40P01"} // deadlock_detected
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (2 retries), got %d", calls)
	}
}

func TestExecInTxWithRetry_DoesNotRetryOnCanceled(t *testing.T) {
	d := newTestDataForRetry()
	var calls int32
	err := d.ExecInTxWithRetry(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", calls)
	}
}

func TestExecInTxWithRetry_DoesNotRetryOnConflict(t *testing.T) {
	d := newTestDataForRetry()
	var calls int32
	conflictErr := apierror.Conflict(apierror.DomainData, "duplicate")
	err := d.ExecInTxWithRetry(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return conflictErr
	})
	if !errors.Is(err, conflictErr) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry on conflict), got %d", calls)
	}
}

func TestExecInTxWithRetry_RetriesOnInternal(t *testing.T) {
	d := newTestDataForRetry()
	var calls int32
	err := d.ExecInTxWithRetry(context.Background(), func(ctx context.Context) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			return apierror.Internal(apierror.DomainData, "transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (1 retry), got %d", calls)
	}
}

func TestExecInTxWithRetry_ExhaustsRetries(t *testing.T) {
	d := newTestDataForRetry()
	var calls int32
	err := d.ExecInTxWithRetry(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return apierror.Internal(apierror.DomainData, "persistent")
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	// 1 initial + 3 retries = 4 total attempts.
	if calls != 4 {
		t.Fatalf("expected 4 calls (1 + 3 retries), got %d", calls)
	}
}

func TestExecInTxWithRetry_RespectsCallerCancel(t *testing.T) {
	d := newTestDataForRetry()
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before first attempt.
	cancel()
	var calls int32
	err := d.ExecInTxWithRetry(ctx, func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected 0 calls (caller cancelled), got %d", calls)
	}
}

func TestIsRetryableDBError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"apierror Internal", apierror.Internal(apierror.DomainData, "x"), true},
		{"apierror Conflict", apierror.Conflict(apierror.DomainData, "x"), false},
		{"apierror BadRequest", apierror.BadRequest(apierror.DomainData, "x"), false},
		{"apierror NotFound", apierror.NotFound(apierror.DomainData, "x"), false},
		{"postgres deadlock", &pq.Error{Code: "40P01"}, true},
		{"postgres serialization failure", &pq.Error{Code: "40001"}, true},
		{"postgres unique violation", &pq.Error{Code: "23505"}, false},
		{"generic error", errors.New("something"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableDBError(tt.err); got != tt.want {
				t.Errorf("isRetryableDBError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestExecInTxWithRetry_BackoffProgression verifies that retries use
// exponential backoff. We can't assert exact durations (time.After is
// imprecise), but we can verify the total time is at least
// 1s+2s = 3s for 3 retries of a persistent error.
func TestExecInTxWithRetry_BackoffProgression(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping backoff timing test in short mode")
	}
	d := newTestDataForRetry()
	start := time.Now()
	_ = d.ExecInTxWithRetry(context.Background(), func(ctx context.Context) error {
		return apierror.Internal(apierror.DomainData, "persistent")
	})
	elapsed := time.Since(start)
	// 3 retries with 1s, 2s, 4s backoff = 7s minimum.
	// Allow some slack for timer imprecision.
	if elapsed < 6*time.Second {
		t.Fatalf("expected >=6s for 3 retries with backoff, got %v", elapsed)
	}
}
