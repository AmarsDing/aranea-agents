package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// fakeGraphRunRepo is a minimal in-memory GraphRunRepo for unit tests.
type fakeGraphRunRepo struct {
	saved   *GraphExecution
	updates []*GraphExecution
}

func (f *fakeGraphRunRepo) SaveRun(_ context.Context, exec *GraphExecution) error {
	f.saved = exec
	return nil
}

func (f *fakeGraphRunRepo) GetRun(_ context.Context, _ string) (*GraphExecution, error) {
	return f.saved, nil
}

func (f *fakeGraphRunRepo) ListRunsByGraph(_ context.Context, _ string, _ int, _ string, _ ...GraphRunListOption) ([]*GraphExecution, string, error) {
	return nil, "", nil
}

func (f *fakeGraphRunRepo) UpdateRun(_ context.Context, exec *GraphExecution) error {
	f.updates = append(f.updates, exec)
	return nil
}

func newTestGraphExecUsecase(repo GraphRunRepo) *GraphExecutionUsecase {
	return &GraphExecutionUsecase{
		sm:         NewGraphExecutionStateMachine(),
		lg:         loggateway.NewNoop(),
		runRepo:    repo,
		executions: make(map[string]*GraphExecution),
	}
}

// TestUpdateExecutionFromRuntimeEvent_RetryingNodeError_KeepsRunning verifies that
// an intermediate node error emitted while the framework is retrying the node
// (Retrying=true) must NOT transition the execution to Failed — the retry may
// still succeed and the graph may complete normally.
func TestUpdateExecutionFromRuntimeEvent_RetryingNodeError_KeepsRunning(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-retry", "graph-1", "sess-1", string(GraphExecRunning))

	uc.updateExecutionFromRuntimeEvent(exec, GraphRuntimeEvent{
		Type:       DomainEventGraphNodeError,
		NodeID:     "node-1",
		Error:      "transient llm timeout",
		StepNumber: 1,
		Retrying:   true,
	})

	if got := exec.GetStatus(); got != string(GraphExecRunning) {
		t.Errorf("retrying node error must not fail execution: Status = %q, want %q", got, GraphExecRunning)
	}
	if len(repo.updates) == 0 {
		t.Error("retrying node error should still persist step snapshot via UpdateRun")
	}
}

// TestUpdateExecutionFromRuntimeEvent_FinalNodeError_FailsExecution verifies that
// a non-retrying (final) node error still transitions the execution to Failed.
func TestUpdateExecutionFromRuntimeEvent_FinalNodeError_FailsExecution(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-final", "graph-1", "sess-1", string(GraphExecRunning))

	uc.updateExecutionFromRuntimeEvent(exec, GraphRuntimeEvent{
		Type:       DomainEventGraphNodeError,
		NodeID:     "node-1",
		Error:      "permanent failure",
		StepNumber: 1,
		Retrying:   false,
	})

	if got := exec.GetStatus(); got != string(GraphExecFailed) {
		t.Errorf("final node error must fail execution: Status = %q, want %q", got, GraphExecFailed)
	}
}

// TestConsumeRuntimeEvents_RetryThenSuccess_CompletesExecution is the B1 regression
// test: node fails once with retry, retry succeeds, graph stream ends normally —
// the execution must end in Completed, not stuck in Failed.
func TestConsumeRuntimeEvents_RetryThenSuccess_CompletesExecution(t *testing.T) {
	repo := &fakeGraphRunRepo{}
	uc := newTestGraphExecUsecase(repo)
	exec := NewGraphExecution(context.Background(), "exec-e2e", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 3)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphNodeError, NodeID: "node-1", Error: "transient", StepNumber: 1, Retrying: true}
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphNodeEnd, NodeID: "node-1", StepNumber: 2}
	// N1 contract: the framework emits an explicit done event on success.
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphDone}
	close(eventCh)

	uc.consumeRuntimeEvents(eventCh, exec, exec.ID, exec.GraphID, exec.SessionID, nil)

	if got := exec.GetStatus(); got != string(GraphExecCompleted) {
		t.Errorf("retry-then-success must complete: Status = %q, want %q", got, GraphExecCompleted)
	}
}
