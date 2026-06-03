package data

import (
	"context"
	"sync"
)

type readinessState int

const (
	readinessPending readinessState = iota
	readinessReady
	readinessFailed
)

// ReadinessGate tracks startup readiness with three states: pending, ready, failed.
// Once MarkReady or MarkFailed is called, the gate is sealed and cannot transition again.
type ReadinessGate struct {
	ready  chan struct{}
	once   sync.Once
	state  readinessState
	reason string
	mu     sync.RWMutex
}

func newReadinessGate() *ReadinessGate {
	return &ReadinessGate{ready: make(chan struct{})}
}

// MarkReady signals that startup completed successfully.
func (g *ReadinessGate) MarkReady() {
	g.once.Do(func() {
		g.mu.Lock()
		g.state = readinessReady
		g.mu.Unlock()
		close(g.ready)
	})
}

// MarkFailed signals that startup failed. The gate will never become ready.
// Subsequent calls to IsReady return false; Wait returns the failure reason.
func (g *ReadinessGate) MarkFailed(reason string) {
	g.once.Do(func() {
		g.mu.Lock()
		g.state = readinessFailed
		g.reason = reason
		g.mu.Unlock()
		close(g.ready)
	})
}

// Wait blocks until the gate is sealed (ready or failed) or the context is cancelled.
func (g *ReadinessGate) Wait(ctx context.Context) error {
	select {
	case <-g.ready:
		g.mu.RLock()
		st := g.state
		g.mu.RUnlock()
		if st == readinessFailed {
			return &readinessFailedError{reason: g.reason}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsReady returns true only if MarkReady was called.
func (g *ReadinessGate) IsReady() bool {
	g.mu.RLock()
	st := g.state
	g.mu.RUnlock()
	return st == readinessReady
}

// IsFailed returns true if MarkFailed was called.
func (g *ReadinessGate) IsFailed() bool {
	g.mu.RLock()
	st := g.state
	g.mu.RUnlock()
	return st == readinessFailed
}

// FailedReason returns the failure reason if IsFailed, otherwise empty string.
func (g *ReadinessGate) FailedReason() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.state == readinessFailed {
		return g.reason
	}
	return ""
}

type readinessFailedError struct {
	reason string
}

func (e *readinessFailedError) Error() string { return "readiness failed: " + e.reason }
