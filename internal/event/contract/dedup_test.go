package contract

import (
	"fmt"
	"sync"
	"testing"
)

func TestEventDeduplicator_NewEvent(t *testing.T) {
	d := NewEventDeduplicator(16)
	if d.IsDuplicate("evt-1") {
		t.Fatal("first sighting should not be duplicate")
	}
}

func TestEventDeduplicator_DuplicateEvent(t *testing.T) {
	d := NewEventDeduplicator(16)
	d.IsDuplicate("evt-1")
	if !d.IsDuplicate("evt-1") {
		t.Fatal("second sighting should be duplicate")
	}
}

func TestEventDeduplicator_EmptyID(t *testing.T) {
	d := NewEventDeduplicator(16)
	if d.IsDuplicate("") {
		t.Fatal("empty ID should not be duplicate")
	}
	if d.IsDuplicate("") {
		t.Fatal("empty ID should not be duplicate on second call")
	}
}

func TestEventDeduplicator_ZeroCapacity(t *testing.T) {
	d := NewEventDeduplicator(0)
	if d.IsDuplicate("evt-1") {
		t.Fatal("zero capacity should disable dedup")
	}
	if d.IsDuplicate("evt-1") {
		t.Fatal("zero capacity should disable dedup")
	}
}

func TestEventDeduplicator_NilReceiver(t *testing.T) {
	var d *EventDeduplicator
	if d.IsDuplicate("evt-1") {
		t.Fatal("nil receiver should not be duplicate")
	}
}

func TestEventDeduplicator_CapacityEviction(t *testing.T) {
	d := NewEventDeduplicator(3)
	d.IsDuplicate("a") // false, add a
	d.IsDuplicate("b") // false, add b
	d.IsDuplicate("c") // false, add c
	// Adding "d" evicts "a" (oldest). Verify d is new.
	if d.IsDuplicate("d") {
		t.Fatal("d should be new, not duplicate")
	}
	// b, c, d should still be in cache (duplicates — no modification).
	if !d.IsDuplicate("b") {
		t.Fatal("b should still be in cache")
	}
	if !d.IsDuplicate("c") {
		t.Fatal("c should still be in cache")
	}
	if !d.IsDuplicate("d") {
		t.Fatal("d should still be in cache")
	}
	// "a" was evicted; checking it returns false (not duplicate).
	// Note: this re-adds "a", evicting "b", but that's expected behavior.
	if d.IsDuplicate("a") {
		t.Fatal("a should have been evicted")
	}
}

func TestEventDeduplicator_Reset(t *testing.T) {
	d := NewEventDeduplicator(16)
	d.IsDuplicate("evt-1")
	d.Reset()
	if d.IsDuplicate("evt-1") {
		t.Fatal("after reset, evt-1 should not be duplicate")
	}
}

func TestEventDeduplicator_Concurrent(t *testing.T) {
	d := NewEventDeduplicator(1024)
	var wg sync.WaitGroup
	// 50 goroutines, each with a unique prefix, checking 20 IDs.
	// Total: 1000 unique IDs, fits in 1024 capacity — no eviction.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				id := fmt.Sprintf("evt-%d-%d", g, j)
				if d.IsDuplicate(id) {
					t.Errorf("goroutine %d: first call for %s should not be duplicate", g, id)
				}
				// Second call should be duplicate (ID still in cache).
				if !d.IsDuplicate(id) {
					t.Errorf("goroutine %d: second call for %s should be duplicate", g, id)
				}
			}
		}(i)
	}
	wg.Wait()
}
