package cronrunner

import (
	"testing"
	"time"
)

func TestRetryBackoffSchedule(t *testing.T) {
	if len(defaultRetryBackoff) != 3 {
		t.Fatalf("expected 3 backoff steps, got %d", len(defaultRetryBackoff))
	}
	if defaultRetryBackoff[0] != 30*time.Second {
		t.Fatalf("first backoff should be 30s, got %v", defaultRetryBackoff[0])
	}
	if defaultRetryBackoff[1] != 2*time.Minute {
		t.Fatalf("second backoff should be 2m, got %v", defaultRetryBackoff[1])
	}
	if defaultRetryBackoff[2] != 10*time.Minute {
		t.Fatalf("third backoff should be 10m, got %v", defaultRetryBackoff[2])
	}
}

func TestMaxDeadFailures(t *testing.T) {
	if maxDeadFailures != 3 {
		t.Fatalf("expected maxDeadFailures=3, got %d", maxDeadFailures)
	}
}

func TestRetryPlan_DefaultAttempts(t *testing.T) {
	attempts, backoff := retryPlan(defaultRetryMaxAttempts)
	if attempts != 4 {
		t.Fatalf("expected 4 attempts with default backoff, got %d", attempts)
	}
	if len(backoff) != 3 {
		t.Fatalf("expected 3 backoff steps, got %d", len(backoff))
	}
}

func TestRetryPlan_DisableRetry(t *testing.T) {
	attempts, backoff := retryPlan(0)
	if attempts != 1 || len(backoff) != 0 {
		t.Fatalf("expected single attempt without backoff, got attempts=%d backoff=%d", attempts, len(backoff))
	}
}

func TestRetryPlan_SingleRetry(t *testing.T) {
	attempts, backoff := retryPlan(1)
	if attempts != 2 || len(backoff) != 1 {
		t.Fatalf("expected 2 attempts and 1 backoff step, got attempts=%d backoff=%d", attempts, len(backoff))
	}
}

func TestEffectiveRetryMaxAttempts(t *testing.T) {
	if got := effectiveRetryMaxAttempts(cronTaskConfig{}); got != defaultRetryMaxAttempts {
		t.Fatalf("missing field should default to %d, got %d", defaultRetryMaxAttempts, got)
	}
	zero := 0
	if got := effectiveRetryMaxAttempts(cronTaskConfig{RetryMaxAttempts: &zero}); got != 0 {
		t.Fatalf("explicit zero should disable retry, got %d", got)
	}
	one := 1
	if got := effectiveRetryMaxAttempts(cronTaskConfig{RetryMaxAttempts: &one}); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestCronTaskMetadata_DeadLetterThreshold(t *testing.T) {
	meta := cronTaskMetadata{FailureCount: maxDeadFailures - 1}
	if meta.FailureCount >= maxDeadFailures {
		t.Fatal("should not be dead yet")
	}
	meta.FailureCount++
	if meta.FailureCount < maxDeadFailures {
		t.Fatal("should have reached dead threshold")
	}
}
