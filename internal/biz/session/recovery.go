package session

import (
	"context"
	"time"
)

func (uc *SessionUsecase) BatchTransitionInterrupted(ctx context.Context, reason SessionStatusReason) error {
	sessions, err := uc.sessionReader.ListSessionsForBatch(ctx, SessionSearchQuery{
		Status: string(SessionStatusRunning),
	})
	if err != nil {
		return err
	}
	interrupted := string(SessionStatusInterrupted)
	reasonStr := string(reason)
	changedAt := time.Now().UTC().Format(time.RFC3339)
	for _, s := range sessions {
		_, _ = uc.sessionWriter.UpdateSession(ctx, s.ID, SessionUpdateFields{
			Status:          &interrupted,
			StatusReason:    &reasonStr,
			StatusChangedAt: &changedAt,
		})
	}
	return nil
}

func (uc *SessionUsecase) RecoverOrphanedRunningSessions(ctx context.Context) error {
	return uc.BatchTransitionInterrupted(ctx, StatusReasonUnexpectedShutdown)
}
