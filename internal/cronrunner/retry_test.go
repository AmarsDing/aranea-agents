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
