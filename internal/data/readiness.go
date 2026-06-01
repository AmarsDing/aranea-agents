package data

import (
	"context"
	"sync"
)

type ReadinessGate struct {
	ready chan struct{}
	once  sync.Once
}

func newReadinessGate() *ReadinessGate {
	return &ReadinessGate{ready: make(chan struct{})}
}

func (g *ReadinessGate) MarkReady() {
	g.once.Do(func() { close(g.ready) })
}

func (g *ReadinessGate) Wait(ctx context.Context) error {
	select {
	case <-g.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *ReadinessGate) IsReady() bool {
	select {
	case <-g.ready:
		return true
	default:
		return false
	}
}
