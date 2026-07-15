package event

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
)

// V2Bus is an in-process fan-out bus for v2 Events.
// Subscribers receive all events; filtering is done by subscribers (e.g. by
// SpiritSessionID).
//
// This implements biz.EventBus for the v2 Sequencer's publish path.
//
// Drop policy (B-06):
//   - Non-terminal events: non-blocking send; full buffer → drop + DropCount++
//   - Terminal events (*.completed/*.failed/*.cancelled/*.interrupted/*.skipped):
//     BlockUpTo (ctx deadline or 2s) before drop, so critical lifecycle state
//     is not silently lost when a subscriber is briefly slow.
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

const terminalPublishBlock = 2 * time.Second

// Publish broadcasts an event to all subscribers.
//
// Subscriber channel snapshot is taken under RLock, then the lock is released
// before any blocking send. Holding RLock across BlockUpTo would deadlock if a
// slow subscriber's handler called Subscribe/cancel (needs Lock).
func (b *V2Bus) Publish(ctx context.Context, e biz.Event) {
	if e == nil {
		return
	}
	critical := biz.IsCriticalDeliveryEvent(e)
	b.mu.RLock()
	subs := make([]chan biz.Event, 0, len(b.subscribers))
	for _, ch := range b.subscribers {
		subs = append(subs, ch)
	}
	b.mu.RUnlock()
	for _, ch := range subs {
		if critical {
			b.publishCritical(ctx, ch, e)
			continue
		}
		select {
		case ch <- e:
		default:
			b.dropped.Add(1)
		}
	}
}

func (b *V2Bus) publishCritical(ctx context.Context, ch chan biz.Event, e biz.Event) {
	select {
	case ch <- e:
		return
	default:
	}
	// Buffer full: BlockUpTo before dropping (AS-EVT-01 Critical).
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := terminalPublishBlock
	if d, ok := ctx.Deadline(); ok {
		if rem := time.Until(d); rem > 0 && rem < deadline {
			deadline = rem
		}
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case ch <- e:
	case <-timer.C:
		b.dropped.Add(1)
	case <-ctx.Done():
		b.dropped.Add(1)
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
