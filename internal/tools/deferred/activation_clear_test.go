package deferred

import "testing"

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
