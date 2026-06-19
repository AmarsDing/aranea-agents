package jobs

import (
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// Circuit breaker thresholds for the sleep-time consolidation job.
//
// When the consolidation target (memory read/write) is unavailable, the
// circuit opens to avoid hammering the DB with retries. After a cool-down
// period it enters half-open and allows a single probe; if the probe
// succeeds the circuit closes, otherwise it re-opens.
const (
	// circuitBreakerFailureThreshold is the number of consecutive failures
	// that trips the circuit from Closed → Open.
	circuitBreakerFailureThreshold = 5
	// circuitBreakerOpenDuration is the cool-down period before the circuit
	// transitions from Open → Half-Open.
	circuitBreakerOpenDuration = 5 * time.Minute
)

// circuitBreakerState enumerates the circuit breaker states.
type circuitBreakerState int

const (
	circuitBreakerClosed   circuitBreakerState = iota // normal operation
	circuitBreakerOpen                                // tripped, rejecting calls
	circuitBreakerHalfOpen                            // probing with a single call
)

// CircuitBreaker implements a simple failure-threshold circuit breaker for
// the JobRunner. It is safe for concurrent use.
//
// State transitions:
//   - Closed  → Open:     after circuitBreakerFailureThreshold consecutive failures
//   - Open    → HalfOpen: after circuitBreakerOpenDuration has elapsed
//   - HalfOpen → Closed:  on a successful probe
//   - HalfOpen → Open:    on a failed probe (resets the cool-down timer)
//
// The circuit breaker is optional — when nil, JobRunner.Run behaves as before
// (no circuit breaking, only retry).
type CircuitBreaker struct {
	mu               sync.Mutex
	state            circuitBreakerState
	consecutiveFails int
	openedAt         time.Time
	lg               loggateway.Logger
}

// NewCircuitBreaker creates a CircuitBreaker with the standard thresholds.
func NewCircuitBreaker(lg loggateway.Logger) *CircuitBreaker {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &CircuitBreaker{
		state: circuitBreakerClosed,
		lg:    lg,
	}
}

// Allow reports whether a job execution is permitted. When the circuit is
// Open, it returns false unless the cool-down has elapsed (in which case it
// transitions to HalfOpen and returns true to allow a single probe).
func (cb *CircuitBreaker) Allow() bool {
	if cb == nil {
		return true
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case circuitBreakerClosed:
		return true
	case circuitBreakerOpen:
		if time.Since(cb.openedAt) >= circuitBreakerOpenDuration {
			cb.state = circuitBreakerHalfOpen
			cb.lg.Info("circuit breaker: open → half-open (probing)")
			return true
		}
		return false
	case circuitBreakerHalfOpen:
		// Allow only one probe at a time; subsequent callers are rejected
		// until the probe completes (RecordSuccess/RecordFailure).
		return false
	}
	return true
}

// RecordSuccess resets the failure counter and closes the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	if cb == nil {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != circuitBreakerClosed {
		cb.lg.Info("circuit breaker: half-open → closed (probe succeeded)")
	}
	cb.state = circuitBreakerClosed
	cb.consecutiveFails = 0
}

// RecordFailure increments the failure counter. When the threshold is reached
// the circuit opens. In HalfOpen state, a single failure re-opens the circuit.
func (cb *CircuitBreaker) RecordFailure() {
	if cb == nil {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFails++
	switch cb.state {
	case circuitBreakerHalfOpen:
		cb.tripOpen()
	case circuitBreakerClosed:
		if cb.consecutiveFails >= circuitBreakerFailureThreshold {
			cb.tripOpen()
		}
	}
}

// tripOpen transitions the circuit to the Open state. Caller must hold cb.mu.
func (cb *CircuitBreaker) tripOpen() {
	cb.state = circuitBreakerOpen
	cb.openedAt = time.Now()
	cb.lg.Warn("circuit breaker: → open (failures tripped)",
		loggateway.Int("consecutive_fails", cb.consecutiveFails),
		loggateway.Any("cool_down", circuitBreakerOpenDuration.String()))
}

// State returns the current circuit breaker state (for diagnostics/metrics).
func (cb *CircuitBreaker) State() circuitBreakerState {
	if cb == nil {
		return circuitBreakerClosed
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
