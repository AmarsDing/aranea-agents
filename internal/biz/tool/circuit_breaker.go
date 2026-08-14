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
	FailureThreshold   int `json:"failure_threshold"`
	RecoveryTimeoutSec int `json:"recovery_timeout_sec"`
	HalfOpenMaxProbe   int `json:"half_open_max_probe"`
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
	halfOpenProbes  int
	probeClaimedAt  time.Time
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
	var from, to CircuitState
	changed := false
	var allowed bool
	var st CircuitState

	switch cb.state {
	case CircuitClosed:
		allowed, st = true, cb.state
	case CircuitOpen:
		if time.Since(cb.lastFailureTime) > cb.config.recoveryTimeout() {
			from, to = cb.state, CircuitHalfOpen
			cb.state = CircuitHalfOpen
			cb.successes = 0
			cb.failures = 0
			cb.halfOpenProbes = 1
			cb.probeClaimedAt = time.Now()
			changed = true
			allowed, st = true, cb.state
		} else {
			allowed, st = false, cb.state
		}
	case CircuitHalfOpen:
		// Recover abandoned probe slots: if the caller that claimed a slot never
		// reports RecordSuccess/RecordFailure (e.g. its future was cancelled), the
		// slot would leak and deadlock the breaker in HalfOpen. Reclaim one slot
		// per Allow() call once the claim has aged past the recovery timeout.
		if cb.halfOpenProbes > 0 && !cb.probeClaimedAt.IsZero() &&
			time.Since(cb.probeClaimedAt) > cb.config.recoveryTimeout() {
			cb.halfOpenProbes--
			cb.probeClaimedAt = time.Time{}
		}
		// Reserve a probe slot atomically: increment halfOpenProbes as a
		// dedicated probe counter so that concurrent Allow() calls cannot
		// exceed HalfOpenMaxProbe. This is separate from successes to avoid
		// RecordSuccess() polluting the probe slot counter.
		if cb.halfOpenProbes < cb.config.HalfOpenMaxProbe {
			cb.halfOpenProbes++
			cb.probeClaimedAt = time.Now()
			allowed, st = true, cb.state
		} else {
			allowed, st = false, cb.state
		}
	default:
		allowed, st = true, cb.state
	}
	cb.mu.Unlock()
	if changed {
		cb.emitStateChange(from, to)
	}
	return allowed, st
}

// RecordSuccess records a successful call. In Closed state, this resets the
// consecutive failure counter to zero (consecutive-failure semantics, not sliding-window).
// In HalfOpen state, this increments the success probe counter and transitions
// to Closed when HalfOpenMaxProbe successes are reached.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	var from, to CircuitState
	changed := false

	switch cb.state {
	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.HalfOpenMaxProbe {
			from = cb.state
			cb.state = CircuitClosed
			cb.failures = 0
			cb.successes = 0
			cb.halfOpenProbes = 0
			cb.probeClaimedAt = time.Time{}
			to = cb.state
			changed = true
		}
	case CircuitClosed:
		cb.failures = 0
		cb.lastFailureTime = time.Time{}
	}
	cb.mu.Unlock()
	if changed {
		cb.emitStateChange(from, to)
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	var from, to CircuitState
	changed := false

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitHalfOpen:
		from = cb.state
		cb.state = CircuitOpen
		cb.successes = 0
		cb.halfOpenProbes = 0
		cb.probeClaimedAt = time.Time{}
		to = cb.state
		changed = true
	case CircuitClosed:
		if cb.failures >= cb.config.FailureThreshold {
			from = cb.state
			cb.state = CircuitOpen
			to = cb.state
			changed = true
		}
	}
	cb.mu.Unlock()
	if changed {
		cb.emitStateChange(from, to)
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	prev := cb.state
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenProbes = 0
	cb.probeClaimedAt = time.Time{}
	changed := prev != CircuitClosed
	cb.mu.Unlock()
	if changed {
		cb.emitStateChange(prev, CircuitClosed)
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
		HalfOpenProbes:  cb.halfOpenProbes,
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
	// Do not restore in-flight probe claims: they belonged to the dead process
	// and their RecordSuccess/RecordFailure will never arrive. probeClaimedAt is
	// not persisted, so restoring halfOpenProbes would leave the HalfOpen reclaim
	// path in Allow() (which requires a non-zero claim timestamp) unable to fire,
	// deadlocking the breaker in half_open with Allow() permanently false.
	cb.halfOpenProbes = 0
	cb.probeClaimedAt = time.Time{}
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
