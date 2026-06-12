package tool

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreakerConfig struct {
	FailureThreshold  int `json:"failure_threshold"`
	RecoveryTimeoutSec int `json:"recovery_timeout_sec"`
	HalfOpenMaxProbe  int `json:"half_open_max_probe"`
}

func (c CircuitBreakerConfig) recoveryTimeout() time.Duration {
	if c.RecoveryTimeoutSec <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.RecoveryTimeoutSec) * time.Second
}

type CircuitBreakerOption func(*CircuitBreaker)

func WithOnStateChange(fn func(name string, from, to CircuitState)) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.onStateChange = fn
	}
}

type CircuitBreaker struct {
	mu              sync.Mutex
	name            string
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	config          CircuitBreakerConfig
	onStateChange   func(name string, from, to CircuitState)
}

func NewCircuitBreaker(name string, cfg CircuitBreakerConfig, opts ...CircuitBreakerOption) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.RecoveryTimeoutSec <= 0 {
		cfg.RecoveryTimeoutSec = 30
	}
	if cfg.HalfOpenMaxProbe <= 0 {
		cfg.HalfOpenMaxProbe = 1
	}
	cb := &CircuitBreaker{
		name:   name,
		state:  CircuitClosed,
		config: cfg,
	}
	for _, opt := range opts {
		opt(cb)
	}
	return cb
}

// Allow checks whether a call is permitted. Side effect: when the breaker is
// in Open state and the recovery timeout has elapsed, Allow transitions the
// state to HalfOpen and resets counters. Callers should treat Allow as a
// query-with-side-effect, not a pure read.
func (cb *CircuitBreaker) Allow() (bool, CircuitState) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true, cb.state
	case CircuitOpen:
		if time.Since(cb.lastFailureTime) > cb.config.recoveryTimeout() {
			prev := cb.state
			cb.state = CircuitHalfOpen
			cb.successes = 0
			cb.failures = 0
			cb.emitStateChange(prev, cb.state)
			return true, cb.state
		}
		return false, cb.state
	case CircuitHalfOpen:
		// Reserve a probe slot atomically: increment successes as a probe
		// counter so that concurrent Allow() calls cannot exceed HalfOpenMaxProbe.
		if cb.successes < cb.config.HalfOpenMaxProbe {
			cb.successes++
			return true, cb.state
		}
		return false, cb.state
	default:
		return true, cb.state
	}
}

// RecordSuccess records a successful call. In Closed state, this resets the
// consecutive failure counter to zero (consecutive-failure semantics, not sliding-window).
// In HalfOpen state, this increments the success probe counter and transitions
// to Closed when HalfOpenMaxProbe successes are reached.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.HalfOpenMaxProbe {
			prev := cb.state
			cb.state = CircuitClosed
			cb.failures = 0
			cb.successes = 0
			cb.emitStateChange(prev, cb.state)
		}
	case CircuitClosed:
		cb.failures = 0
		cb.lastFailureTime = time.Time{}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitHalfOpen:
		prev := cb.state
		cb.state = CircuitOpen
		cb.successes = 0
		cb.emitStateChange(prev, cb.state)
	case CircuitClosed:
		if cb.failures >= cb.config.FailureThreshold {
			prev := cb.state
			cb.state = CircuitOpen
			cb.emitStateChange(prev, cb.state)
		}
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	prev := cb.state
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	if prev != CircuitClosed {
		cb.emitStateChange(prev, cb.state)
	}
}

// Name returns the circuit breaker's tool name. The name field is set only at
// construction time and never mutated, so no lock is required.
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

func (cb *CircuitBreaker) emitStateChange(from, to CircuitState) {
	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, from, to)
	}
}

func (cb *CircuitBreaker) UpdateConfig(cfg CircuitBreakerConfig) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.RecoveryTimeoutSec <= 0 {
		cfg.RecoveryTimeoutSec = 30
	}
	if cfg.HalfOpenMaxProbe <= 0 {
		cfg.HalfOpenMaxProbe = 1
	}
	cb.config = cfg
}

// snapshotEntry returns the current breaker state as a CircuitBreakerStateEntry
// for persistence. Caller must NOT hold cb.mu.
func (cb *CircuitBreaker) snapshotEntry() CircuitBreakerStateEntry {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return CircuitBreakerStateEntry{
		State:           string(cb.state),
		FailureCount:    cb.failures,
		SuccessCount:    cb.successes,
		LastFailureTime: cb.lastFailureTime,
		UpdatedAt:       time.Now(),
	}
}

// restoreFromEntry applies a persisted state entry to this breaker.
// This is used during startup to recover from a process restart.
// Caller must NOT hold cb.mu.
func (cb *CircuitBreaker) restoreFromEntry(entry CircuitBreakerStateEntry) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitState(entry.State)
	cb.failures = entry.FailureCount
	cb.successes = entry.SuccessCount
	cb.lastFailureTime = entry.LastFailureTime
}

func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}
