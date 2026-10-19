package event

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
)

// CriticalDelegate handles a critical-delivery event that arrived at
// V2Bus.Publish directly from a producer OTHER than the v2 Sequencer
// (e.g. service.PublishRunStatusFull, the session status publisher,
// pause/resume gateways). The v2 Sequencer registers itself as the delegate
// (see v2.NewSequencer) so such events still receive a delivery Seq and a
// durable event_delivery_outbox row (AS-EVT-01). Implementations must fan
// out via V2Bus.Fanout (never Publish) to avoid delegate recursion.
type CriticalDelegate func(ctx context.Context, e biz.Event)

// criticalDelegateHolder wraps the delegate so nil can be stored in an
// atomic.Value with a consistent concrete type.
type criticalDelegateHolder struct{ d CriticalDelegate }

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
//   - With a CriticalDelegate set, critical events arriving via Publish are
//     handed to the delegate instead of the bus-local BlockUpTo lane; the
//     delegate owns outbox persistence + fan-out for them.
type V2Bus struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan biz.Event
	nextID      uint64
	dropped     atomic.Uint64
	// journal optionally persists critical events to JSONL (B-06). Nil = no-op.
	journal *CriticalJournal
	// criticalDelegate, when set, intercepts critical events in Publish.
	// Written once at bootstrap (Sequencer construction) before publishers
	// run; atomically loaded on every Publish.
	criticalDelegate atomic.Value // stores criticalDelegateHolder
}

// SetCriticalDelegate installs the critical-delivery delegate (nil clears).
// Typically called exactly once at bootstrap by the v2 Sequencer over this
// bus; last registration wins.
func (b *V2Bus) SetCriticalDelegate(d CriticalDelegate) {
	b.criticalDelegate.Store(criticalDelegateHolder{d: d})
}

// NewV2Bus creates a new in-process V2Bus without a durable critical journal.
func NewV2Bus() *V2Bus {
	return &V2Bus{subscribers: make(map[uint64]chan biz.Event)}
}

// NewV2BusWithJournal creates a V2Bus that best-effort appends critical events
// to journal before fan-out. journal may be nil (same as NewV2Bus).
func NewV2BusWithJournal(journal *CriticalJournal) *V2Bus {
	return &V2Bus{
		subscribers: make(map[uint64]chan biz.Event),
		journal:     journal,
	}
}

const terminalPublishBlock = 2 * time.Second

// Publish broadcasts an event to all subscribers.
//
// Subscriber channel snapshot is taken under RLock, then the lock is released
// before any blocking send. Holding RLock across BlockUpTo would deadlock if a
// slow subscriber's handler called Subscribe/cancel (needs Lock).
//
// Sends to a channel that was detached by a concurrent cancel after the
// snapshot are safe: subscriber channels are never closed (see Subscribe), so
// an in-flight send either lands in the detached buffer (GC'd later) or times
// out per the drop policy.
func (b *V2Bus) Publish(ctx context.Context, e biz.Event) {
	if e == nil {
		return
	}
	// Critical events published directly by non-Sequencer producers are
	// delegated (Seq assignment + durable outbox row + fan-out); the
	// Sequencer's own fan-out path uses Fanout and never re-enters this hook.
	if biz.IsCriticalDeliveryEvent(e) {
		if h, ok := b.criticalDelegate.Load().(criticalDelegateHolder); ok && h.d != nil {
			h.d(ctx, e)
			return
		}
	}
	b.Fanout(ctx, e)
}

// Fanout broadcasts an event to all subscribers without consulting the
// critical delegate. The v2 Sequencer calls this after handling the critical
// delivery itself (outbox insert + Seq assignment), so events routed through
// the delegate are fanned out exactly once.
func (b *V2Bus) Fanout(ctx context.Context, e biz.Event) {
	if e == nil {
		return
	}
	critical := biz.IsCriticalDeliveryEvent(e)
	if critical && b.journal != nil {
		_ = b.journal.Append(e) // best-effort; never block publish on disk errors
	}
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
			b.recordDrop(e, "nonterminal_buffer_full")
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
		b.recordDrop(e, "terminal_blockup_timeout")
	case <-ctx.Done():
		b.recordDrop(e, "terminal_ctx_cancelled")
	}
}

// recordDrop counts a subscriber-buffer drop both in the in-memory DropCount
// and the exported Prometheus metric (P1-Y7). The policy label distinguishes
// the drop path: nonterminal_buffer_full | terminal_blockup_timeout |
// terminal_ctx_cancelled.
func (b *V2Bus) recordDrop(e biz.Event, policy string) {
	b.dropped.Add(1)
	metrics.EventBusDropped.WithLabelValues(string(e.EventKind()), policy).Inc()
}

// Subscribe registers a subscriber and returns a channel + cancel function.
// The subscriber buffer defaults to 256.
//
// The bus NEVER closes the returned channel: cancel only detaches the
// subscriber from the fan-out set. Closing under a concurrent Publish that
// has already snapshotted the channel panics with "send on closed channel"
// (R-1 close-race); leaving the channel open lets the in-flight send complete
// or time out harmlessly, and the detached channel is GC'd once the
// subscriber's own loop exits. Subscribers must therefore exit their receive
// loop via their own context (红线 #23), not via channel close.
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
		delete(b.subscribers, id)
	}
	return ch, cancel
}

// DropCount returns the total number of events dropped due to full subscriber buffers.
func (b *V2Bus) DropCount() uint64 {
	return b.dropped.Load()
}

var _ biz.EventBus = (*V2Bus)(nil)
