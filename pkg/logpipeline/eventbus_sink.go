package logpipeline

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

type Publisher interface {
	Publish(ctx context.Context, kind EntryKind, level, message, sessionID string, fields map[string]any)
}

// Circuit breaker states
const (
	cbClosed   = iota // normal operation
	cbOpen            // circuit open, all writes skipped
	cbHalfOpen        // probing if downstream is healthy
)

const (
	cbFailureThreshold = 5                // consecutive failures before opening
	cbOpenDuration     = 10 * time.Second // how long to stay open
	cbHalfOpenMaxProbe = 3                // successful probes needed to close
)

type EventBusSink struct {
	pub     Publisher
	level   logLevel
	author  string
	channel string

	// Circuit breaker fields (lock-free via atomics)
	failures        atomic.Int64
	state           atomic.Int32 // cbClosed/cbOpen/cbHalfOpen
	openUntil       atomic.Int64 // unix nano timestamp
	halfOpenSuccess atomic.Int64
	dropped         atomic.Uint64
}

func NewEventBusSink(pub Publisher, hookLevel string) *EventBusSink {
	return &EventBusSink{
		pub:     pub,
		level:   parseLogLevel(hookLevel),
		author:  "system",
		channel: "monitor",
	}
}

func (s *EventBusSink) Write(entry LogEntry) {
	if s == nil || s.pub == nil {
		return
	}
	entryLevel := parseLogLevel(entry.Level)
	if entryLevel < s.level {
		return
	}

	// Circuit breaker check
	state := s.state.Load()
	switch state {
	case cbOpen:
		if time.Now().UnixNano() < s.openUntil.Load() {
			s.dropped.Add(1)
			return
		}
		// Transition to half-open
		s.state.Store(cbHalfOpen)
		s.halfOpenSuccess.Store(0)
	case cbHalfOpen:
		// Allow probe through
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	s.pub.Publish(ctx, entry.Kind, entry.Level, entry.Message, entry.SessionID, entry.Fields)

	// Check if publish succeeded (context timeout = failure)
	if ctx.Err() != nil {
		s.onFailure()
	} else {
		s.onSuccess()
	}
}

func (s *EventBusSink) onFailure() {
	failures := s.failures.Add(1)
	state := s.state.Load()

	if state == cbHalfOpen {
		// Probe failed, go back to open
		s.state.Store(cbOpen)
		s.openUntil.Store(time.Now().Add(cbOpenDuration).UnixNano())
		s.halfOpenSuccess.Store(0)
		s.failures.Store(0)
		fmt.Fprintf(os.Stderr, "[eventbus_sink] circuit breaker: half-open probe failed, re-opening for %v\n", cbOpenDuration)
		return
	}

	if failures >= cbFailureThreshold {
		s.state.Store(cbOpen)
		s.openUntil.Store(time.Now().Add(cbOpenDuration).UnixNano())
		s.failures.Store(0)
		fmt.Fprintf(os.Stderr, "[eventbus_sink] circuit breaker: %d consecutive failures, opening for %v\n", cbFailureThreshold, cbOpenDuration)
	}
}

func (s *EventBusSink) onSuccess() {
	state := s.state.Load()
	if state == cbHalfOpen {
		probes := s.halfOpenSuccess.Add(1)
		if probes >= cbHalfOpenMaxProbe {
			s.state.Store(cbClosed)
			s.failures.Store(0)
			s.halfOpenSuccess.Store(0)
		}
	} else if state == cbClosed {
		s.failures.Store(0)
	}
}

func (s *EventBusSink) Flush() {}

func (s *EventBusSink) Close() error { return nil }

func (s *EventBusSink) Dropped() uint64 {
	return s.dropped.Load()
}
