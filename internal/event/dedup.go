package event

import (
	"sync"
)

// DefaultDedupCapacity is the default capacity for EventDeduplicator.
// Critical events (ToolResult/Error/RunnerCompletion/Checkpoint) are rare
// relative to Informational events; 4096 entries comfortably cover the WAL
// recovery window where duplicates may arrive.
const DefaultDedupCapacity = 4096

// EventDeduplicator tracks seen event IDs to filter duplicates.
//
// Originally introduced to handle WAL recovery replays (AS-EVT-01 post-publish
// failure scenario); with WAL/event_store removed in Phase 1c-2 the replay
// path no longer exists. Retained as a defensive measure against accidental
// double-publish by upstream producers — subscribers of Critical event types
// (Error/Checkpoint) use IsDuplicate to skip the second delivery.
//
// The deduplicator uses a bounded ring buffer with O(1) lookup via a companion
// map. When capacity is reached, the oldest entries are evicted (FIFO).
//
// Thread-safe.
//
// Stability:evolving
type EventDeduplicator struct {
	mu       sync.Mutex
	seen     map[string]struct{}
	ring     []string
	pos      int // next write position in ring
	capacity int
}

// NewEventDeduplicator creates a deduplicator with the given capacity.
// A capacity of 0 disables deduplication (IsDuplicate always returns false).
func NewEventDeduplicator(capacity int) *EventDeduplicator {
	if capacity <= 0 {
		return &EventDeduplicator{capacity: 0}
	}
	return &EventDeduplicator{
		seen:     make(map[string]struct{}, capacity),
		ring:     make([]string, capacity),
		pos:      0,
		capacity: capacity,
	}
}

// IsDuplicate returns true if the eventID has been seen before.
// If not, it marks the eventID as seen and returns false.
// Empty eventIDs are never considered duplicates (defensive — allows
// events without IDs to pass through without filtering).
func (d *EventDeduplicator) IsDuplicate(eventID string) bool {
	if d == nil || d.capacity == 0 || eventID == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[eventID]; ok {
		return true
	}
	// Evict oldest if at capacity (ring is full).
	if len(d.seen) >= d.capacity {
		old := d.ring[d.pos]
		if old != "" {
			delete(d.seen, old)
		}
	}
	d.seen[eventID] = struct{}{}
	d.ring[d.pos] = eventID
	d.pos = (d.pos + 1) % d.capacity
	return false
}

// Reset clears all seen event IDs.
func (d *EventDeduplicator) Reset() {
	if d == nil || d.capacity == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = make(map[string]struct{}, d.capacity)
	d.ring = make([]string, d.capacity)
	d.pos = 0
}
