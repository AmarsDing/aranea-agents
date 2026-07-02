package v2

import (
	"context"

	"aranea-agents/internal/biz"
)

// eventBusAdapter wraps a biz.EventBus to satisfy the local EventBus interface.
//
// The local EventBus interface (in sequencer.go) is structurally identical to
// biz.EventBus — both expose Publish and Subscribe with the same signatures.
// This adapter exists for explicitness and to provide a seam for future
// v2-specific extensions (e.g. subscribe options, metrics) without requiring
// callers to depend on biz.EventBus directly.
//
// Stability: evolving — Phase 1 helper for integration tests and the
// standalone Sequencer wiring (see Task 17 integration test).
type eventBusAdapter struct {
	bus biz.EventBus
}

// Publish delegates to the wrapped biz.EventBus.
func (a *eventBusAdapter) Publish(ctx context.Context, e biz.Event) {
	a.bus.Publish(ctx, e)
}

// Subscribe delegates to the wrapped biz.EventBus.
func (a *eventBusAdapter) Subscribe(opts biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return a.bus.Subscribe(opts)
}

// NewEventBusAdapter wraps a biz.EventBus implementation (e.g. event.V2Bus or
// a test recordingBus) so it satisfies the local EventBus interface required
// by NewSequencer.
func NewEventBusAdapter(bus biz.EventBus) EventBus {
	return &eventBusAdapter{bus: bus}
}
