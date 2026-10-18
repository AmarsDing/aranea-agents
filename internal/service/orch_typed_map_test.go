package service

import (
	"testing"
	"time"
)

// Live entry wins (loaded=true) and the proposed value is discarded.
func TestTypedSyncMap_LoadOrStore_LiveEntryWins(t *testing.T) {
	m := NewTypedSyncMap[string, int](time.Hour)
	m.Store("k", 1)

	v, loaded := m.LoadOrStore("k", 2)
	if !loaded || v != 1 {
		t.Fatalf("expected live entry to win with loaded=true, v=1; got loaded=%v v=%d", loaded, v)
	}
}

// First store claims the key (loaded=false).
func TestTypedSyncMap_LoadOrStore_FirstStoreClaims(t *testing.T) {
	m := NewTypedSyncMap[string, int](time.Hour)

	v, loaded := m.LoadOrStore("k", 1)
	if loaded || v != 1 {
		t.Fatalf("expected first store to claim with loaded=false, v=1; got loaded=%v v=%d", loaded, v)
	}
}

// Review #5: an expired entry must NOT win LoadOrStore — the stale guard
// value would otherwise block forever (e.g. resumeInFlight) even though
// Load/Sweep already treat the entry as gone.
func TestTypedSyncMap_LoadOrStore_ExpiredEntryReplaced(t *testing.T) {
	m := NewTypedSyncMap[string, int](10 * time.Millisecond)
	m.Store("k", 1)
	time.Sleep(30 * time.Millisecond)

	v, loaded := m.LoadOrStore("k", 2)
	if loaded || v != 2 {
		t.Fatalf("expired entry must lose: want loaded=false, v=2; got loaded=%v v=%d", loaded, v)
	}
	if got, ok := m.Load("k"); !ok || got != 2 {
		t.Fatalf("new value must be stored after replacing expired entry; got ok=%v v=%d", ok, got)
	}
}

// ttl<=0 disables expiry: LoadOrStore keeps legacy loaded semantics.
func TestTypedSyncMap_LoadOrStore_NoTTLKeepsLegacy(t *testing.T) {
	m := NewTypedSyncMap[string, int](0)
	m.Store("k", 1)

	v, loaded := m.LoadOrStore("k", 2)
	if !loaded || v != 1 {
		t.Fatalf("ttl=0 must never expire: want loaded=true, v=1; got loaded=%v v=%d", loaded, v)
	}
}

// Load drops expired entries (pre-existing behavior, locked for parity with
// the LoadOrStore fix).
func TestTypedSyncMap_Load_ExpiredEntryMiss(t *testing.T) {
	m := NewTypedSyncMap[string, int](10 * time.Millisecond)
	m.Store("k", 1)
	time.Sleep(30 * time.Millisecond)

	if _, ok := m.Load("k"); ok {
		t.Fatal("Load must miss on expired entry")
	}
}
