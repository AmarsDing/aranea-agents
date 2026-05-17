// Package testutil provides lightweight test helpers for Aranea unit tests.
// It is intentionally free of dependencies on internal/data or internal/data/ent
// to avoid the ent init-time panic that currently affects most integration test suites.
//
// NOTE: The ent runtime.go panics on init due to a nil default value at line 1500
// (session.DefaultContextUsedRatio type assertion).  Until that generated file is
// regenerated with a schema fix, any test that transitively imports the ent package
// will panic.  Tests in this package are safe because they do NOT import ent.
package testutil

import (
	"context"
	"sync"

	"aranea-agents/internal/event"
)

// RecordingBus is an in-memory event.Bus that records all published envelopes.
// Use it in tests that need to assert on published events without a real bus.
type RecordingBus struct {
	mu     sync.Mutex
	events []event.Envelope
}

var _ event.Bus = (*RecordingBus)(nil)

func NewRecordingBus() *RecordingBus { return &RecordingBus{} }

func (b *RecordingBus) Publish(_ context.Context, env event.Envelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, env)
}

func (b *RecordingBus) Subscribe(_ event.SubscribeOptions) (<-chan event.Envelope, func()) {
	ch := make(chan event.Envelope, 32)
	return ch, func() { close(ch) }
}

func (b *RecordingBus) DropCount() uint64 { return 0 }

// Events returns a copy of all published envelopes.
func (b *RecordingBus) Events() []event.Envelope {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]event.Envelope, len(b.events))
	copy(cp, b.events)
	return cp
}

// EventsOfType returns envelopes whose Type matches the given value.
func (b *RecordingBus) EventsOfType(t event.EnvelopeType) []event.Envelope {
	all := b.Events()
	var out []event.Envelope
	for _, e := range all {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}
