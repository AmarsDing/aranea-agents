package biz

import (
	"testing"
)

func TestTurnPreviewRegistry_Register(t *testing.T) {
	reg := NewTurnPreviewRegistry()
	firstStopped := false
	secondStopped := false
	_ = reg.Register("sess-1", func() { firstStopped = true })
	stop2 := reg.Register("sess-1", func() { secondStopped = true })
	if !firstStopped {
		t.Fatal("previous preview should stop when replaced")
	}
	stop2()
	if !secondStopped {
		t.Fatal("second stop should run")
	}
}

func TestTurnPreviewRegistry_SetRunID(t *testing.T) {
	reg := NewTurnPreviewRegistry()
	reg.Register("sess-1", func() {})
	reg.SetRunID("sess-1", "run-a")
	if got := reg.ActiveRunID("sess-1"); got != "run-a" {
		t.Fatalf("runID=%q", got)
	}
}

func TestTurnPreviewRegistry_Unregister(t *testing.T) {
	reg := NewTurnPreviewRegistry()
	reg.Register("sess-1", func() {})
	reg.Unregister("sess-1")
	if got := reg.ActiveRunID("sess-1"); got != "" {
		t.Fatalf("expected empty runID after unregister, got %q", got)
	}
}

func TestTurnPreviewRegistry_EmptySessionID(t *testing.T) {
	reg := NewTurnPreviewRegistry()
	cancel := reg.Register("", func() {})
	if cancel == nil {
		t.Fatal("should return cancel func even for empty sessionID")
	}
	reg.SetRunID("", "run-a")
	if got := reg.ActiveRunID(""); got != "" {
		t.Fatalf("expected empty for empty sessionID, got %q", got)
	}
}

func TestTurnPreviewRegistry_NilReceiver(t *testing.T) {
	var r *TurnPreviewRegistry
	cancel := r.Register("sess-1", func() {})
	if cancel == nil {
		t.Fatal("should return cancel func for nil receiver")
	}
	r.Unregister("sess-1")
	r.SetRunID("sess-1", "run-a")
	if got := r.ActiveRunID("sess-1"); got != "" {
		t.Fatalf("expected empty for nil receiver, got %q", got)
	}
}
