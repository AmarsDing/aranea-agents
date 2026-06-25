package event

import (
	"context"

	"aranea-agents/pkg/loggateway"
	frameworkbus "trpc.group/trpc-go/trpc-agent-go/event/bus"
)

// GenericBus is a type-safe wrapper around frameworkbus.Bus[T].
// It allows creating buses for any event type without import cycles,
// enabling the ActivityEventBus (which transports biz.ActivityEvent)
// to be created in packages that import biz.
type GenericBus[T any] struct {
	inner frameworkbus.Bus[T]
	lg    loggateway.Logger
}

// GenericSubscribeOptions configures a GenericBus subscription.
type GenericSubscribeOptions[T any] struct {
	BufferSize int
	Filter     func(T) bool
}

// NewGenericBus creates a new GenericBus for the given event type.
func NewGenericBus[T any](lg loggateway.Logger) *GenericBus[T] {
	var dropLogger frameworkbus.DropLogger[T]
	if lg != nil {
		dropLogger = func(event T, policy string, totalDrops uint64) {
			lg.Warn("[generic_bus] drop",
				loggateway.Str("policy", policy),
				loggateway.Int64("total_drops", int64(totalDrops)),
			)
		}
	}
	opts := []frameworkbus.Option[T]{}
	if dropLogger != nil {
		opts = append(opts, frameworkbus.WithDropLogger(dropLogger))
	}
	return &GenericBus[T]{
		inner: frameworkbus.New[T](opts...),
		lg:    lg,
	}
}

// Publish broadcasts an event to all matching subscribers.
func (b *GenericBus[T]) Publish(ctx context.Context, event T) {
	b.inner.Publish(ctx, event)
}

// Subscribe registers a subscriber and returns a channel of events.
func (b *GenericBus[T]) Subscribe(opts GenericSubscribeOptions[T]) (<-chan T, func()) {
	fwOpts := frameworkbus.SubscribeOptions[T]{
		BufferSize: opts.BufferSize,
	}
	if opts.Filter != nil {
		fwOpts.Filter = opts.Filter
	}
	return b.inner.Subscribe(fwOpts)
}

// DropCount returns the total number of dropped events.
func (b *GenericBus[T]) DropCount() uint64 {
	return b.inner.DropCount()
}
