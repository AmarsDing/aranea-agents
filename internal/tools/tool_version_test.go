package tools

import "testing"

func TestResolveRegistration_BehaviorVersion(t *testing.T) {
	regs := []*ToolRegistration{
		{Name: "read", BehaviorVersion: 1},
		{Name: "read", BehaviorVersion: 2},
		{Name: "write"}, // unversioned → treated as version 1
	}

	// Session pinned at v1 always resolves v1, even after v2 is registered.
	if got := ResolveRegistration(regs, "read", 1); got == nil || got.BehaviorVersion != 1 {
		t.Fatalf("expected behavior version 1, got %+v", got)
	}
	if got := ResolveRegistration(regs, "read", 2); got == nil || got.BehaviorVersion != 2 {
		t.Fatalf("expected behavior version 2, got %+v", got)
	}
	// version 0 = unpinned → latest registered version.
	if got := ResolveRegistration(regs, "read", 0); got == nil || got.BehaviorVersion != 2 {
		t.Fatalf("expected latest version 2, got %+v", got)
	}
	// Unversioned registrations resolve as version 1.
	if got := ResolveRegistration(regs, "write", 1); got == nil {
		t.Fatal("expected unversioned registration to resolve as version 1")
	}
	// Unknown name / unavailable version → nil.
	if got := ResolveRegistration(regs, "nonexistent", 0); got != nil {
		t.Fatalf("expected nil for unknown tool, got %+v", got)
	}
	if got := ResolveRegistration(regs, "read", 99); got != nil {
		t.Fatalf("expected nil for unavailable version, got %+v", got)
	}
}

func TestLatestBehaviorVersion(t *testing.T) {
	regs := []*ToolRegistration{
		{Name: "read", BehaviorVersion: 1},
		{Name: "read", BehaviorVersion: 3},
		{Name: "read", BehaviorVersion: 2},
		{Name: "write"},
	}
	if got := LatestBehaviorVersion(regs, "read"); got != 3 {
		t.Fatalf("expected latest version 3, got %d", got)
	}
	if got := LatestBehaviorVersion(regs, "write"); got != 1 {
		t.Fatalf("expected latest version 1 for unversioned, got %d", got)
	}
	if got := LatestBehaviorVersion(regs, "missing"); got != 0 {
		t.Fatalf("expected 0 for missing tool, got %d", got)
	}
}
