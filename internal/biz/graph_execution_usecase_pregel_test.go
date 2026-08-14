package biz

import (
	"context"
	"strings"
	"testing"
)

// N1: stream ends without a framework completion event (fatal graph error was
// the last thing on the wire, or the stream terminated early) — the execution
// must converge to Failed, NOT Completed (fail-closed).
func TestConsumeRuntimeEvents_StreamEndWithoutDone_FailsExecution(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-nodone", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 1)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphNodeEnd, NodeID: "node-1", StepNumber: 1}
	close(eventCh)

	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, nil)

	if got := exec.GetStatus(); got != string(GraphExecFailed) {
		t.Fatalf("stream end without done must fail: Status = %q, want %q", got, GraphExecFailed)
	}
	if exec.ErrorMessage == "" {
		t.Fatal("ErrorMessage must explain the missing completion event")
	}
}

// N1: an explicit framework done event keeps the success path intact.
func TestConsumeRuntimeEvents_DoneEvent_CompletesExecution(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-done", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 2)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphNodeEnd, NodeID: "node-1", StepNumber: 1}
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphDone}
	close(eventCh)

	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, nil)

	if got := exec.GetStatus(); got != string(GraphExecCompleted) {
		t.Fatalf("done event must complete: Status = %q, want %q", got, GraphExecCompleted)
	}
}

// N1: a graph-level execution error event (Pregel fatal) fails the execution
// immediately and persists the failed snapshot.
func TestUpdateExecutionFromRuntimeEvent_ExecutionError_FailsExecution(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-err", "graph-1", "sess-1", string(GraphExecRunning))

	uc.updateExecutionFromRuntimeEvent(exec, 0, GraphRuntimeEvent{
		Type:  DomainEventGraphExecutionError,
		Error: "graph execution exceeded max steps",
	})

	if got := exec.GetStatus(); got != string(GraphExecFailed) {
		t.Fatalf("execution error must fail: Status = %q, want %q", got, GraphExecFailed)
	}
	if !strings.Contains(exec.ErrorMessage, "max steps") {
		t.Fatalf("ErrorMessage = %q, want to contain %q", exec.ErrorMessage, "max steps")
	}
	if len(repo.updates) == 0 {
		t.Fatal("execution error must persist the failed snapshot")
	}
}

// N2: after an interrupt the stream closes — WaitingHuman is not Running, so
// consumeRuntimeEvents must leave the interrupted execution untouched.
func TestConsumeRuntimeEvents_InterruptThenStreamEnd_StaysWaitingHuman(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-hitl", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 1)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphInterrupt, NodeID: "review-1", StepNumber: 2}
	close(eventCh)

	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, nil)

	if got := exec.GetStatus(); got != string(GraphExecWaitingHuman) {
		t.Fatalf("interrupted execution must stay waiting_human: Status = %q", got)
	}
}

// Y2 (stream generation): after Resume starts a new stream, the stale stream's
// consumer must NOT converge terminal state — otherwise the old stream ending
// (without a done event) would falsely fail the new, still-running stream.
func TestConsumeRuntimeEvents_StaleStream_SkipsTerminalConvergence(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-stale", "graph-1", "sess-1", string(GraphExecRunning))
	// Simulate: stream gen 1 was consuming; Resume bumped the generation to 2
	// and a new consumer took over. The exec is Running under the NEW stream.
	exec.streamGen = 2

	eventCh := make(chan GraphRuntimeEvent, 1)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphNodeEnd, NodeID: "node-1", StepNumber: 1}
	close(eventCh)

	completedCalled := false
	uc.consumeRuntimeEvents(eventCh, exec, 1, exec.ID, exec.GraphID, exec.SessionID, func() { completedCalled = true })

	if got := exec.GetStatus(); got != string(GraphExecRunning) {
		t.Fatalf("stale stream must not change status: Status = %q, want %q", got, GraphExecRunning)
	}
	if exec.ErrorMessage != "" {
		t.Fatalf("stale stream must not set ErrorMessage, got %q", exec.ErrorMessage)
	}
	if completedCalled {
		t.Fatal("stale stream must not invoke onComplete (the new stream owns completion)")
	}
	for _, u := range repo.updates {
		if u.Status != string(GraphExecRunning) {
			t.Fatalf("stale stream must not persist terminal status, got %q", u.Status)
		}
	}
}

// Y2: a stale stream's graph-level fatal error event must not fail the new stream.
func TestUpdateExecutionFromRuntimeEvent_StaleStream_IgnoresExecutionError(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-stale-err", "graph-1", "sess-1", string(GraphExecRunning))
	exec.streamGen = 2

	uc.updateExecutionFromRuntimeEvent(exec, 1, GraphRuntimeEvent{
		Type:  DomainEventGraphExecutionError,
		Error: "stale pregel fatal",
	})

	if got := exec.GetStatus(); got != string(GraphExecRunning) {
		t.Fatalf("stale execution error must not fail: Status = %q, want %q", got, GraphExecRunning)
	}
	if exec.ErrorMessage != "" {
		t.Fatalf("stale execution error must not set ErrorMessage, got %q", exec.ErrorMessage)
	}
}

// Y2: a stale stream's late interrupt event must not pause the new stream.
func TestUpdateExecutionFromRuntimeEvent_StaleStream_IgnoresInterrupt(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-stale-hitl", "graph-1", "sess-1", string(GraphExecRunning))
	exec.streamGen = 2

	uc.updateExecutionFromRuntimeEvent(exec, 1, GraphRuntimeEvent{
		Type:   DomainEventGraphInterrupt,
		NodeID: "review-1",
	})

	if got := exec.GetStatus(); got != string(GraphExecRunning) {
		t.Fatalf("stale interrupt must not pause: Status = %q, want %q", got, GraphExecRunning)
	}
	if exec.GetInterruptNode() != "" {
		t.Fatalf("stale interrupt must not set InterruptNode, got %q", exec.GetInterruptNode())
	}
}

// Y2: a stale stream's final node error must not fail the new stream.
func TestUpdateExecutionFromRuntimeEvent_StaleStream_IgnoresFinalNodeError(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-stale-nodeerr", "graph-1", "sess-1", string(GraphExecRunning))
	exec.streamGen = 2

	uc.updateExecutionFromRuntimeEvent(exec, 1, GraphRuntimeEvent{
		Type:     DomainEventGraphNodeError,
		NodeID:   "node-1",
		Error:    "stale node failure",
		Retrying: false,
	})

	if got := exec.GetStatus(); got != string(GraphExecRunning) {
		t.Fatalf("stale final node error must not fail: Status = %q, want %q", got, GraphExecRunning)
	}
}
