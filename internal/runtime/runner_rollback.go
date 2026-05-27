package runtime

import "context"

// RunnerSessionRollbackStore records framework-session boundaries so a failed
// turn can remove events written after admission without deleting UI artifacts.
type RunnerSessionRollbackStore interface {
	MarkBoundary(ctx context.Context, sessionID, runID, turnID string) (boundaryID string, err error)
	RollbackToBoundary(ctx context.Context, sessionID, boundaryID string) error
}

type RunnerRollbackBoundary struct {
	SessionID  string
	RunID      string
	TurnID     string
	BoundaryID string
}

func (m *RunnerManager) MarkRollbackBoundary(ctx context.Context, sessionID, runID, turnID string) (RunnerRollbackBoundary, error) {
	if m == nil || m.factory.Persist.RunnerRollback == nil {
		return RunnerRollbackBoundary{SessionID: sessionID, RunID: runID, TurnID: turnID}, nil
	}
	id, err := m.factory.Persist.RunnerRollback.MarkBoundary(ctx, sessionID, runID, turnID)
	if err != nil {
		return RunnerRollbackBoundary{}, err
	}
	return RunnerRollbackBoundary{SessionID: sessionID, RunID: runID, TurnID: turnID, BoundaryID: id}, nil
}

func (m *RunnerManager) RollbackToBoundary(ctx context.Context, boundary RunnerRollbackBoundary) error {
	if m == nil || m.factory.Persist.RunnerRollback == nil || boundary.BoundaryID == "" {
		return nil
	}
	return m.factory.Persist.RunnerRollback.RollbackToBoundary(ctx, boundary.SessionID, boundary.BoundaryID)
}
