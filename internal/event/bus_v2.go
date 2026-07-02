package event

import (
	"context"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
)

// V2Bus is an in-process fan-out bus for v2 Events.
// Subscribers receive all events; filtering is done by subscribers (e.g. by
// SpiritSessionID).
//
// This implements biz.EventBus for the v2 Sequencer's publish path.
// Non-blocking sends: if a subscriber's buffer is full, the event is dropped
// for that subscriber (counted via DropCount).
type V2Bus struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan biz.Event
	nextID      uint64
	dropped     atomic.Uint64
}

// NewV2Bus creates a new in-process V2Bus.
func NewV2Bus() *V2Bus {
	return &V2Bus{subscribers: make(map[uint64]chan biz.Event)}
}

// Publish broadcasts an event to all subscribers.
// Non-blocking: drops the event for slow subscribers.
func (b *V2Bus) Publish(_ context.Context, e biz.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
			b.dropped.Add(1)
		}
	}
}

// Subscribe registers a subscriber and returns a channel + cancel function.
// The subscriber buffer defaults to 256.
func (b *V2Bus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	ch := make(chan biz.Event, 256)
	b.subscribers[id] = ch
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subscribers[id]; ok {
			close(c)
			delete(b.subscribers, id)
		}
	}
	return ch, cancel
}

// DropCount returns the total number of events dropped due to full subscriber buffers.
func (b *V2Bus) DropCount() uint64 {
	return b.dropped.Load()
}

var _ biz.EventBus = (*V2Bus)(nil)
