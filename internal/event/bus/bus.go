// Package bus provides a generic, in-process publish-subscribe event bus.
//
// It supports multiple subscribers with configurable delivery policies
// (DropOldest, DropNewest, BlockUpTo), priority-based delivery, and
// flexible filtering. The bus is safe for concurrent use.
//
// Type parameter T allows using any type as the event payload
// (e.g., *event.Event, a custom Envelope struct, or any domain type).
package bus

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DropPolicy controls what happens when a subscriber's buffer is full.
type DropPolicy int

const (
	// DropOldest evicts the oldest event in the buffer to make room for the new one.
	DropOldest DropPolicy = iota
	// DropNewest silently discards the new event when the buffer is full.
	DropNewest
	// BlockUpTo blocks for a configurable duration before falling back to DropOldest.
	BlockUpTo DropPolicy = 2
)

// ChannelPriority controls subscription priority.
type ChannelPriority int

const (
	// PriorityCritical subscribers receive events before PriorityNormal subscribers.
	PriorityCritical ChannelPriority = iota
	// PriorityNormal subscribers receive events after PriorityCritical subscribers.
	PriorityNormal
)

const (
	// DefaultBufferSize is the default subscriber channel capacity.
	DefaultBufferSize = 128
	// MaxBufferSize is the maximum allowed subscriber channel capacity.
	MaxBufferSize = 512
)

// EventMatcher is a function that checks if an event matches a subscription filter.
// Return true to deliver the event, false to skip it.
type EventMatcher[T any] func(event T) bool

// SubscribeOptions configures a single Bus subscription.
type SubscribeOptions[T any] struct {
	// Priority controls delivery order. PriorityCritical subscribers receive
	// events before PriorityNormal subscribers.
	Priority ChannelPriority

	// BufferSize is the capacity of the subscriber's channel.
	// Default: DefaultBufferSize, Max: MaxBufferSize.
	BufferSize int

	// Reliable forces BlockUpTo delivery policy regardless of the event's
	// reliability tier. Useful for subscribers that must never miss events.
	Reliable bool

	// DropPolicy controls what happens when the subscriber's buffer is full.
	DropPolicy DropPolicy

	// BlockFor is the duration to block when using BlockUpTo policy
	// before falling back to DropOldest. Default: 100ms.
	BlockFor time.Duration

	// Filter is an optional function that filters events before delivery.
	// Return true to deliver, false to skip.
	Filter EventMatcher[T]
}

// DropLogger is called when an event is dropped due to back-pressure.
type DropLogger[T any] func(event T, policy string, totalDrops uint64)

// Bus is the in-process event fanout hub interface.
// Implementations must be safe for concurrent use.
type Bus[T any] interface {
	// Publish sends an event to all matching subscribers.
	Publish(ctx context.Context, event T)
	// Subscribe registers a new subscriber with the given options.
	// Returns a read-only channel and an unsubscribe function.
	Subscribe(opts SubscribeOptions[T]) (<-chan T, func())
	// DropCount returns the total number of dropped events.
	DropCount() uint64
}

// bus is the default in-process Bus implementation.
type bus[T any] struct {
	mu          sync.RWMutex
	subscribers map[uint64]*subscriber[T]
	nextID      uint64
	dropCount   atomic.Uint64
	logDrop     DropLogger[T]
}

type subscriber[T any] struct {
	mu     sync.RWMutex
	ch     chan T
	opts   SubscribeOptions[T]
	closed bool
}

// Option configures a new Bus.
type Option[T any] func(*bus[T])

// WithDropLogger sets a custom drop logger.
func WithDropLogger[T any](logger DropLogger[T]) Option[T] {
	return func(b *bus[T]) {
		b.logDrop = logger
	}
}

// New creates a new in-process event bus.
func New[T any](opts ...Option[T]) Bus[T] {
	b := &bus[T]{
		subscribers: make(map[uint64]*subscriber[T]),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Publish sends an event to all matching subscribers.
// Critical-priority subscribers receive the event before Normal-priority ones.
func (b *bus[T]) Publish(ctx context.Context, event T) {
	b.mu.RLock()
	criticalSubs := make([]*subscriber[T], 0, len(b.subscribers))
	normalSubs := make([]*subscriber[T], 0, len(b.subscribers))
	for _, sub := range b.subscribers {
		if sub.opts.Priority == PriorityCritical {
			criticalSubs = append(criticalSubs, sub)
		} else {
			normalSubs = append(normalSubs, sub)
		}
	}
	b.mu.RUnlock()

	for _, sub := range criticalSubs {
		b.deliverToSubscriber(sub, event)
	}
	for _, sub := range normalSubs {
		b.deliverToSubscriber(sub, event)
	}
}

// Subscribe registers a new subscriber with the given options.
// Returns a read-only channel and an unsubscribe function.
func (b *bus[T]) Subscribe(opts SubscribeOptions[T]) (<-chan T, func()) {
	bufSize := opts.BufferSize
	if bufSize <= 0 {
		bufSize = DefaultBufferSize
	}
	if bufSize > MaxBufferSize {
		bufSize = MaxBufferSize
	}
	ch := make(chan T, bufSize)
	id := atomic.AddUint64(&b.nextID, 1)
	b.mu.Lock()
	b.subscribers[id] = &subscriber[T]{ch: ch, opts: opts}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		sub, ok := b.subscribers[id]
		if !ok {
			b.mu.Unlock()
			return
		}
		delete(b.subscribers, id)
		b.mu.Unlock()

		sub.mu.Lock()
		if !sub.closed {
			sub.closed = true
		}
		sub.mu.Unlock()
	}
	return ch, unsubscribe
}

// DropCount returns the total number of dropped events.
func (b *bus[T]) DropCount() uint64 {
	return b.dropCount.Load()
}

func (b *bus[T]) deliverToSubscriber(sub *subscriber[T], event T) {
	sub.mu.RLock()
	closed := sub.closed
	sub.mu.RUnlock()
	if closed {
		return
	}

	policy := sub.opts.DropPolicy
	blockFor := sub.opts.BlockFor

	if sub.opts.Reliable {
		policy = BlockUpTo
		if blockFor <= 0 {
			blockFor = 100 * time.Millisecond
		}
	}

	// Apply filter if set
	if sub.opts.Filter != nil && !sub.opts.Filter(event) {
		return
	}

	switch policy {
	case BlockUpTo:
		b.deliverBlockUpTo(sub, event, blockFor)
	case DropNewest:
		b.deliverDropNewest(sub, event)
	default:
		b.deliverDropOldest(sub, event)
	}
}

func (b *bus[T]) deliverBlockUpTo(sub *subscriber[T], event T, blockFor time.Duration) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	select {
	case sub.ch <- event:
		return
	default:
	}
	if blockFor <= 0 {
		blockFor = 100 * time.Millisecond
	}
	timer := time.NewTimer(blockFor)
	defer timer.Stop()
	for {
		select {
		case sub.ch <- event:
			return
		case <-timer.C:
			b.deliverDropOldestLocked(sub, event)
			return
		}
	}
}

func (b *bus[T]) deliverDropOldest(sub *subscriber[T], event T) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	b.deliverDropOldestLocked(sub, event)
}

func (b *bus[T]) deliverDropOldestLocked(sub *subscriber[T], event T) {
	select {
	case sub.ch <- event:
	default:
		select {
		case <-sub.ch:
			select {
			case sub.ch <- event:
			default:
				b.logEventDrop(event, "drop_oldest")
			}
		default:
			b.logEventDrop(event, "drop_oldest")
		}
	}
}

func (b *bus[T]) deliverDropNewest(sub *subscriber[T], event T) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	select {
	case sub.ch <- event:
	default:
		b.logEventDrop(event, "drop_newest")
	}
}

func (b *bus[T]) logEventDrop(event T, policy string) {
	b.dropCount.Add(1)
	if b.logDrop != nil {
		b.logDrop(event, policy, b.dropCount.Load())
	}
}

// MatchLevelFilter checks if a log level matches a minimum level filter.
// This is a utility function for subscribers that want to filter by log level.
func MatchLevelFilter(filter, level string) bool {
	if filter == "" || level == "" {
		return true
	}
	levelOrder := map[string]int{
		"DEBUG": 0,
		"INFO":  1,
		"WARN":  2,
		"ERROR": 3,
	}
	minLevel, ok := levelOrder[strings.ToUpper(filter)]
	if !ok {
		for _, f := range strings.Split(filter, "|") {
			if strings.EqualFold(strings.TrimSpace(f), level) {
				return true
			}
		}
		return false
	}
	eventLevel, ok := levelOrder[strings.ToUpper(level)]
	if !ok {
		return true
	}
	return eventLevel >= minLevel
}
