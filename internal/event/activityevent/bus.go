// Package activityevent provides the ActivityEventBus implementation
// that transports biz.ActivityEvent events. It bridges the generic
// framework bus to the biz.ActivityEventBus interface.
//
// This package exists separately from internal/event to avoid an import
// cycle (biz imports event, so event cannot import biz).
package activityevent

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// Bus implements biz.ActivityEventBus using a generic framework bus.
// It is safe for concurrent use.
type Bus struct {
	inner *event.GenericBus[biz.ActivityEvent]
}

// New creates a new ActivityEventBus.
func New(lg loggateway.Logger) *Bus {
	return &Bus{
		inner: event.NewGenericBus[biz.ActivityEvent](lg),
	}
}

// Publish broadcasts an ActivityEvent to all matching subscribers.
func (b *Bus) Publish(ctx context.Context, ev biz.ActivityEvent) {
	b.inner.Publish(ctx, ev)
}

// Subscribe registers a subscriber that receives ActivityEvents matching
// the given options.
//
// When opts.Filter is set, it is applied directly. Otherwise, when GlobalMode
// is false and SessionID is set, a session-scoped filter is derived. In
// GlobalMode (or when SessionID is empty and no Filter is set), all events
// are delivered.
func (b *Bus) Subscribe(opts biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	genericOpts := event.GenericSubscribeOptions[biz.ActivityEvent]{
		BufferSize: opts.BufferSize,
	}
	if opts.Filter != nil {
		genericOpts.Filter = opts.Filter
	} else if !opts.GlobalMode && opts.SessionID != "" {
		sessionID := opts.SessionID
		genericOpts.Filter = func(ev biz.ActivityEvent) bool {
			return ev.Activity.SessionID == sessionID
		}
	}
	return b.inner.Subscribe(genericOpts)
}

// DropCount returns the total number of dropped events.
func (b *Bus) DropCount() uint64 {
	return b.inner.DropCount()
}

// Compile-time interface check.
var _ biz.ActivityEventBus = (*Bus)(nil)
