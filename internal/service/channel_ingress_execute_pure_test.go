package service

import (
	"context"
	"testing"
	"time"
)

// turnTerminalPersistCtx must keep terminal-state persistence alive after the
// turn context is cancelled/timed out (CH-B2): the derived context must not be
// done when the parent is cancelled, but must still carry parent values.
func TestTurnTerminalPersistCtx_SurvivesParentCancel(t *testing.T) {
	type ctxKey struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "job-123"))
	cancel() // simulate turn timeout: parent already done

	persistCtx, persistCancel := turnTerminalPersistCtx(parent)
	defer persistCancel()

	select {
	case <-persistCtx.Done():
		t.Fatal("persist ctx must not be done after parent cancellation")
	default:
	}
	if got := persistCtx.Value(ctxKey{}); got != "job-123" {
		t.Fatalf("persist ctx lost parent value: %v", got)
	}
	if deadline, ok := persistCtx.Deadline(); !ok {
		t.Fatal("persist ctx must carry a deadline to bound DB writes")
	} else if until := time.Until(deadline); until <= 0 || until > 30*time.Second {
		t.Fatalf("persist ctx deadline out of expected bound: %v", until)
	}
}
