package runtime

import (
	"context"
	"testing"
)

type testRollbackStore struct {
	markedSession string
	rolledBackTo  string
}

func (s *testRollbackStore) MarkBoundary(_ context.Context, sessionID, runID, turnID string) (string, error) {
	s.markedSession = sessionID + ":" + runID + ":" + turnID
	return "boundary-1", nil
}

func (s *testRollbackStore) RollbackToBoundary(_ context.Context, _ string, boundaryID string) error {
	s.rolledBackTo = boundaryID
	return nil
}

func TestRunnerManagerRollbackBoundaryDelegatesToStore(t *testing.T) {
	store := &testRollbackStore{}
	mgr := NewRunnerManager(RunnerFactoryDeps{Persist: PersistenceSet{RunnerRollback: store}})

	boundary, err := mgr.MarkRollbackBoundary(context.Background(), "sess-1", "run-1", "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if boundary.BoundaryID != "boundary-1" || store.markedSession != "sess-1:run-1:turn-1" {
		t.Fatalf("unexpected boundary=%+v store=%+v", boundary, store)
	}
	if err := mgr.RollbackToBoundary(context.Background(), boundary); err != nil {
		t.Fatal(err)
	}
	if store.rolledBackTo != "boundary-1" {
		t.Fatalf("rolledBackTo=%q", store.rolledBackTo)
	}
}
