package processor

import (
	"context"
	"reflect"
	"time"
)

// AutoHealStrategy defines how a component auto-heals from errors.
// Implementations are embedded in flow processors (LLM, MCP, Tool)
// to provide self-healing at the source of the error.
type AutoHealStrategy interface {
	// CanHeal returns true if this strategy can handle the given error.
	CanHeal(err error) bool

	// Heal attempts to recover from the error. The attempt parameter
	// is 1-indexed (first attempt = 1). Returns nil on success.
	Heal(ctx context.Context, attempt int) error

	// MaxAttempts returns the maximum number of heal attempts.
	MaxAttempts() int

	// Backoff returns the duration to wait before the given attempt.
	Backoff(attempt int) time.Duration
}

// AutoHealConfig configures auto-heal behavior for a flow processor.
type AutoHealConfig struct {
	// Enabled controls whether auto-heal is active.
	Enabled bool

	// MaxAttempts is the maximum number of heal attempts per error.
	MaxAttempts int

	// InitialBackoff is the base backoff duration for the first retry.
	InitialBackoff time.Duration

	// BackoffFactor is the multiplier for exponential backoff.
	BackoffFactor float64

	// MaxBackoff caps the backoff duration.
	MaxBackoff time.Duration
}

// DefaultAutoHealConfig returns a sensible default configuration.
func DefaultAutoHealConfig() AutoHealConfig {
	return AutoHealConfig{
		Enabled:        true,
		MaxAttempts:    3,
		InitialBackoff: 2 * time.Second,
		BackoffFactor:  2.0,
		MaxBackoff:     30 * time.Second,
	}
}

// ComputeBackoff calculates the backoff duration for a given attempt
// using exponential backoff with the given config.
func (c AutoHealConfig) ComputeBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return c.InitialBackoff
	}
	d := c.InitialBackoff
	for i := 1; i < attempt; i++ {
		d = time.Duration(float64(d) * c.BackoffFactor)
		if d > c.MaxBackoff {
			return c.MaxBackoff
		}
	}
	if d > c.MaxBackoff {
		return c.MaxBackoff
	}
	return d
}

// AutoHealResult captures the outcome of an auto-heal cycle.
type AutoHealResult struct {
	// Healed is true if the error was successfully recovered.
	Healed bool

	// Strategy is the name of the strategy that was applied.
	Strategy string

	// Attempts is the number of heal attempts made.
	Attempts int

	// FinalError is the error after all attempts, nil if healed.
	FinalError error
}

// ExecuteAutoHeal runs the auto-heal strategy with backoff and returns the result.
// This is a helper function that any processor can use.
func ExecuteAutoHeal(ctx context.Context, strategy AutoHealStrategy, err error) AutoHealResult {
	if !strategy.CanHeal(err) {
		return AutoHealResult{
			Healed:     false,
			Strategy:   "unknown",
			Attempts:   0,
			FinalError: err,
		}
	}

	maxAttempts := strategy.MaxAttempts()
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		backoff := strategy.Backoff(attempt)
		if backoff > 0 {
			select {
			case <-ctx.Done():
				return AutoHealResult{
					Healed:     false,
					Strategy:   strategyName(strategy),
					Attempts:   attempt,
					FinalError: ctx.Err(),
				}
			case <-time.After(backoff):
			}
		}

		if healErr := strategy.Heal(ctx, attempt); healErr == nil {
			return AutoHealResult{
				Healed:   true,
				Strategy: strategyName(strategy),
				Attempts: attempt,
			}
		} else {
			lastErr = healErr
		}
	}

	return AutoHealResult{
		Healed:     false,
		Strategy:   strategyName(strategy),
		Attempts:   maxAttempts,
		FinalError: lastErr,
	}
}

func strategyName(s AutoHealStrategy) string {
	if s == nil {
		return "nil"
	}
	t := reflect.TypeOf(s)
	if t.Kind() == reflect.Ptr {
		return t.Elem().Name()
	}
	return t.Name()
}

// HealCircuitBreaker stops auto-heal after consecutive failures.
type HealCircuitBreaker struct {
	maxConsecutiveFailures int
	resetDuration          time.Duration
	failures               int
	lastFailureTime        time.Time
	open                   bool
}

// NewHealCircuitBreaker creates a circuit breaker that opens after
// maxConsecutiveFailures consecutive failures and resets after resetDuration.
func NewHealCircuitBreaker(maxConsecutiveFailures int, resetDuration time.Duration) *HealCircuitBreaker {
	return &HealCircuitBreaker{
		maxConsecutiveFailures: maxConsecutiveFailures,
		resetDuration:          resetDuration,
	}
}

// RecordSuccess resets the failure counter.
func (cb *HealCircuitBreaker) RecordSuccess() {
	cb.failures = 0
	cb.open = false
}

// RecordFailure increments the failure counter and potentially opens the circuit.
func (cb *HealCircuitBreaker) RecordFailure() {
	cb.failures++
	cb.lastFailureTime = time.Now()
	if cb.failures >= cb.maxConsecutiveFailures {
		cb.open = true
	}
}

// IsOpen returns true if the circuit breaker is open (healing should be skipped).
func (cb *HealCircuitBreaker) IsOpen() bool {
	if !cb.open {
		return false
	}
	// Check if reset duration has passed
	if time.Since(cb.lastFailureTime) > cb.resetDuration {
		cb.open = false
		cb.failures = 0
		return false
	}
	return true
}
