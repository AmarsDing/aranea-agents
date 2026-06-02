package graph

import "testing"

func TestCircuitBreakerStateMachine(t *testing.T) {
	cbState := NewCircuitBreakerState()
	nodeID := "member-1"
	threshold := 3
	cb := cbState.afterNode(nodeID, threshold)
	ctx := t.Context()
	for i := 0; i < threshold; i++ {
		_, _ = cb(ctx, nil, nil, nil, errTestFail)
	}
	if cbState.State(nodeID) != breakerOpen {
		t.Fatalf("expected open after %d failures", threshold)
	}
	_, _ = cb(ctx, nil, nil, nil, nil)
	if cbState.State(nodeID) != breakerHalfOpen {
		t.Fatalf("expected half-open after first success, got %v", cbState.State(nodeID))
	}
	_, _ = cb(ctx, nil, nil, nil, nil)
	if cbState.State(nodeID) != breakerClosed {
		t.Fatalf("expected closed after second success")
	}
}

var errTestFail = testFailErr{}

type testFailErr struct{}

func (testFailErr) Error() string { return "fail" }
