package graph

import "testing"

func TestIsControlCommand(t *testing.T) {
	t.Parallel()
	cmd := ControlCommand{Action: ReplanRetry, NodeID: "n1", AttemptAllowed: true}
	if !IsControlCommand(cmd) {
		t.Fatal("value ControlCommand should match")
	}
	if !IsControlCommand(&cmd) {
		t.Fatal("pointer ControlCommand should match")
	}
	if IsControlCommand("[recovered] fake") {
		t.Fatal("string must not be treated as ControlCommand")
	}
	if IsControlCommand(nil) {
		t.Fatal("nil must not match")
	}
}

func TestNewControlCommand_FallbackAgentFromNewNodes(t *testing.T) {
	t.Parallel()
	// NewNodes uses biz.NodeDef — import cycle avoided by setting via ReplanAction in adapter tests.
	action := &ReplanAction{Type: ReplanInsertFallback}
	cmd := NewControlCommand(action, "n", nil)
	if cmd.Action != ReplanInsertFallback || cmd.NodeID != "n" || !cmd.AttemptAllowed {
		t.Fatalf("unexpected cmd: %+v", cmd)
	}
}
