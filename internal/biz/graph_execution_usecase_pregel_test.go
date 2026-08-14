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

	uc.consumeRuntimeEvents(eventCh, exec, exec.ID, exec.GraphID, exec.SessionID, nil)

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

	uc.consumeRuntimeEvents(eventCh, exec, exec.ID, exec.GraphID, exec.SessionID, nil)

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

	uc.updateExecutionFromRuntimeEvent(exec, GraphRuntimeEvent{
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

	uc.consumeRuntimeEvents(eventCh, exec, exec.ID, exec.GraphID, exec.SessionID, nil)

	if got := exec.GetStatus(); got != string(GraphExecWaitingHuman) {
		t.Fatalf("interrupted execution must stay waiting_human: Status = %q", got)
	}
}
