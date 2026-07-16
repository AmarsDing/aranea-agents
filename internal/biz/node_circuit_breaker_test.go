package biz

import "testing"

func TestCriticLoopCondFuncRefForThreshold(t *testing.T) {
	if got := CriticLoopCondFuncRefForThreshold(0); got != CriticLoopCondFuncRef {
		t.Fatalf("threshold 0: got %q want %q", got, CriticLoopCondFuncRef)
	}
	got := CriticLoopCondFuncRefForThreshold(0.8)
	want := CriticLoopCondFuncRef + "@0.8"
	if got != want {
		t.Fatalf("threshold 0.8: got %q want %q", got, want)
	}
}

func TestNodeCircuitBreakerRegistry_OpensAfterThreshold(t *testing.T) {
	reg := NewNodeCircuitBreakerRegistry()
	pol := &CircuitBreakerPolicy{FailureThreshold: 2, ResetTimeoutSeconds: 60}
	cb := reg.ForNode("team:t1", "member-1", pol)
	if cb == nil {
		t.Fatal("expected breaker")
	}
	cb.RecordFailure()
	allowed, _ := cb.Allow()
	if !allowed {
		t.Fatal("should allow after 1 failure")
	}
	cb.RecordFailure()
	allowed, st := cb.Allow()
	if allowed || string(st) != "open" {
		t.Fatalf("expected open after threshold, allowed=%v state=%s", allowed, st)
	}
	msg := CircuitOpenErrorMessage("member-1", string(st))
	if !IsCircuitOpenError(errString(msg)) {
		t.Fatal("IsCircuitOpenError should match message")
	}
	if IsCircuitOpenError(nil) {
		t.Fatal("nil should not be circuit open")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
