package biz

import "testing"

func TestA2AInvokeLimiter_AllowWithinWindow(t *testing.T) {
	lim := NewA2AInvokeLimiter(2, 0)
	if !lim.Allow("a", "b") || !lim.Allow("a", "b") {
		t.Fatal("expected first two invokes allowed")
	}
	if lim.Allow("a", "b") {
		t.Fatal("expected third invoke blocked")
	}
	if !lim.Allow("a", "c") {
		t.Fatal("expected different callee allowed")
	}
}
