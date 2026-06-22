package tool

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold:   3,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed, got %s", cb.State())
	}
	allowed, _ := cb.Allow()
	if !allowed {
		t.Fatal("should be allowed in closed state")
	}
}

func TestCircuitBreaker_TransitionsToOpen(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold:   3,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open after %d failures, got %s", 3, cb.State())
	}
	allowed, _ := cb.Allow()
	if allowed {
		t.Fatal("should be blocked in open state")
	}
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}
	time.Sleep(1100 * time.Millisecond)
	allowed, state := cb.Allow()
	if !allowed || state != CircuitHalfOpen {
		t.Fatalf("expected half_open after timeout, allowed=%v state=%s", allowed, state)
	}
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after probe success, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailsBackToOpen(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(1100 * time.Millisecond)
	cb.Allow()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open after half-open failure, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenMultipleProbes(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   3,
	})
	// Trip the breaker to Open.
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}

	// Wait for recovery timeout to transition to HalfOpen.
	time.Sleep(1100 * time.Millisecond)

	// First probe should be allowed.
	allowed, state := cb.Allow()
	if !allowed || state != CircuitHalfOpen {
		t.Fatalf("expected half_open probe 1 allowed, allowed=%v state=%s", allowed, state)
	}

	// Second probe should be allowed.
	allowed, state = cb.Allow()
	if !allowed || state != CircuitHalfOpen {
		t.Fatalf("expected half_open probe 2 allowed, allowed=%v state=%s", allowed, state)
	}

	// Third probe should be allowed (reaches HalfOpenMaxProbe).
	allowed, state = cb.Allow()
	if !allowed || state != CircuitHalfOpen {
		t.Fatalf("expected half_open probe 3 allowed, allowed=%v state=%s", allowed, state)
	}

	// Fourth probe should be blocked (exceeds HalfOpenMaxProbe=3).
	allowed, _ = cb.Allow()
	if allowed {
		t.Fatal("expected probe 4 to be blocked (exceeds HalfOpenMaxProbe)")
	}

	// Record 2 successes — not enough to close yet.
	cb.RecordSuccess()
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected still half_open after 2 successes (need 3), got %s", cb.State())
	}

	// Third success should transition to Closed.
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after 3 probe successes, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenMultipleProbes_FailureResets(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   3,
	})
	// Trip the breaker to Open.
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(1100 * time.Millisecond)

	// Allow 2 probes.
	cb.Allow()
	cb.Allow()

	// One success recorded.
	cb.RecordSuccess()

	// Now a failure during HalfOpen should transition back to Open and reset counters.
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open after half-open failure, got %s", cb.State())
	}

	// Wait again for recovery timeout.
	time.Sleep(1100 * time.Millisecond)

	// Should be able to get all 3 probe slots again (halfOpenProbes was reset).
	allowed, state := cb.Allow()
	if !allowed || state != CircuitHalfOpen {
		t.Fatalf("expected half_open after second recovery, allowed=%v state=%s", allowed, state)
	}
	allowed, _ = cb.Allow()
	if !allowed {
		t.Fatal("expected probe 2 allowed after recovery")
	}
	allowed, _ = cb.Allow()
	if !allowed {
		t.Fatal("expected probe 3 allowed after recovery")
	}
	allowed, _ = cb.Allow()
	if allowed {
		t.Fatal("expected probe 4 blocked (exceeds HalfOpenMaxProbe)")
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold:   3,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed (success reset failures), got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold:   1,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}
	cb.Reset()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after reset, got %s", cb.State())
	}
}

func TestCircuitBreakerRegistry_Get(t *testing.T) {
	reg := NewCircuitBreakerRegistry()
	cb := reg.Get("web_research", "web")
	if cb == nil {
		t.Fatal("expected non-nil circuit breaker")
	}
	if cb.Name() != "web_research" {
		t.Fatalf("expected name web_research, got %s", cb.Name())
	}
	same := reg.Get("web_research", "web")
	if cb != same {
		t.Fatal("expected same instance for same tool name")
	}
}

func TestCircuitBreakerRegistry_OpenBreakers(t *testing.T) {
	reg := NewCircuitBreakerRegistry()
	cb1 := reg.Get("tool_a", "web")
	cb2 := reg.Get("tool_b", "web")
	cb1.RecordFailure()
	cb1.RecordFailure()
	cb1.RecordFailure()
	open := reg.OpenBreakers()
	if len(open) != 1 || open[0] != "tool_a" {
		t.Fatalf("expected [tool_a], got %v", open)
	}
	cb2.RecordFailure()
	cb2.RecordFailure()
	cb2.RecordFailure()
	open = reg.OpenBreakers()
	if len(open) != 2 {
		t.Fatalf("expected 2 open breakers, got %d", len(open))
	}
}

func TestCircuitBreakerRegistry_Override(t *testing.T) {
	reg := NewCircuitBreakerRegistry()
	reg.SetOverride("custom_tool", CircuitBreakerConfig{
		FailureThreshold:   5,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   2,
	})
	cb := reg.Get("custom_tool", "web")
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed (threshold=5), got %s", cb.State())
	}
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open after 5 failures, got %s", cb.State())
	}
}

func TestCircuitBreakerConfig_RecoveryTimeoutConversion(t *testing.T) {
	cfg := CircuitBreakerConfig{RecoveryTimeoutSec: 30}
	if cfg.recoveryTimeout() != 30*time.Second {
		t.Fatalf("expected 30s, got %v", cfg.recoveryTimeout())
	}
	cfg = CircuitBreakerConfig{RecoveryTimeoutSec: 0}
	if cfg.recoveryTimeout() != 30*time.Second {
		t.Fatalf("expected default 30s, got %v", cfg.recoveryTimeout())
	}
	cfg = CircuitBreakerConfig{RecoveryTimeoutSec: 5}
	if cfg.recoveryTimeout() != 5*time.Second {
		t.Fatalf("expected 5s, got %v", cfg.recoveryTimeout())
	}
}

func TestIsTransientError(t *testing.T) {
	if IsTransientError(nil) {
		t.Fatal("nil error should not be transient")
	}
	if !IsTransientError(io.EOF) {
		t.Fatal("io.EOF should be transient")
	}
	if !IsTransientError(io.ErrUnexpectedEOF) {
		t.Fatal("io.ErrUnexpectedEOF should be transient")
	}
	if !IsTransientError(context.Canceled) {
		t.Fatal("context.Canceled should be transient")
	}
	if !IsTransientError(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded should be transient")
	}
	var timeoutErr net.Error = &net.OpError{Op: "read", Net: "tcp", Err: &timeoutError{}}
	if !IsTransientError(timeoutErr) {
		t.Fatal("net.Error should be transient")
	}
	persistentErr := errors.New("authentication failed")
	if IsTransientError(persistentErr) {
		t.Fatal("persistent error should not be transient")
	}
}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
