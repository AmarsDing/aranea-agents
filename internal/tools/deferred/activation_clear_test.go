package deferred

import (
	"testing"
	"time"
)

func TestClearActivated_RemovesSessionEntry(t *testing.T) {
	const sid = "hardening-clear-activated-sess"
	t.Cleanup(func() { ClearActivated(sid) })

	registryActivate(sid, "tool_a")
	registryActivate(sid, "tool_b")
	if !registryActivated(sid, "tool_a") || !registryActivated(sid, "tool_b") {
		t.Fatal("expected tools activated before clear")
	}
	before := ActivatedSessionCount()
	if before < 1 {
		t.Fatalf("ActivatedSessionCount=%d want >=1", before)
	}

	ClearActivated(sid)
	if registryActivated(sid, "tool_a") || registryActivated(sid, "tool_b") {
		t.Fatal("expected registry cleared for session")
	}
	ClearActivated("")  // no-op
	ClearActivated(sid) // idempotent
}

func TestClearActivated_DoesNotAffectOtherSessions(t *testing.T) {
	const a, b = "hardening-clear-a", "hardening-clear-b"
	t.Cleanup(func() {
		ClearActivated(a)
		ClearActivated(b)
	})
	registryActivate(a, "x")
	registryActivate(b, "y")
	ClearActivated(a)
	if registryActivated(a, "x") {
		t.Fatal("session a should be cleared")
	}
	if !registryActivated(b, "y") {
		t.Fatal("session b must remain")
	}
}


func TestSweepStaleActivated_RemovesIdle(t *testing.T) {
	sid := "sweep-ttl-sess"
	t.Cleanup(func() { ClearActivated(sid) })
	registryActivate(sid, "web_search")
	set, ok := activatedRegistry.load(sid)
	if !ok {
		t.Fatal("expected registry entry")
	}
	set.mu.Lock()
	set.lastUsed = time.Now().Add(-3 * time.Hour)
	set.mu.Unlock()
	n := SweepStaleActivated(2 * time.Hour)
	if n < 1 {
		t.Fatalf("SweepStaleActivated removed %d, want >=1", n)
	}
	if _, ok := activatedRegistry.load(sid); ok {
		t.Fatal("stale session should be gone")
	}
}
