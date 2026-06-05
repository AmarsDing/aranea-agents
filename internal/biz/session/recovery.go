package session

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"
)

func (uc *SessionUsecase) BatchTransitionInterrupted(ctx context.Context, reason SessionStatusReason) error {
	sessions, err := uc.sessionReader.ListSessionsForBatch(ctx, SessionSearchQuery{
		Status: string(SessionStatusRunning),
	})
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}
	interrupted := string(SessionStatusInterrupted)
	reasonStr := string(reason)
	changedAt := time.Now().UTC().Format(time.RFC3339)
	var failedCount int
	for _, s := range sessions {
		var updateErr error
		if uc.runtimeWriter != nil {
			updateErr = uc.runtimeWriter.TransitionSessionStatus(ctx, s.ID, s.Status, interrupted, reasonStr, changedAt)
		} else {
			_, updateErr = uc.sessionWriter.UpdateSession(ctx, s.ID, SessionUpdateFields{
				Status:          &interrupted,
				StatusReason:    &reasonStr,
				StatusChangedAt: &changedAt,
			})
		}
		if updateErr != nil {
			failedCount++
			uc.lg.Warn("batch transition interrupted: failed to update session",
				loggateway.Str("session_id", s.ID),
				loggateway.Err(updateErr),
			)
		} else if uc.statusPublisher != nil {
			uc.statusPublisher.PublishSessionStatusChanged(s.ID, interrupted, reasonStr, changedAt)
		}
	}
	if failedCount > 0 {
		uc.lg.Warn("batch transition interrupted: some sessions failed",
			loggateway.Int("total", len(sessions)),
			loggateway.Int("failed", failedCount),
		)
	}
	return nil
}

func (uc *SessionUsecase) RecoverOrphanedRunningSessions(ctx context.Context) error {
	return uc.BatchTransitionInterrupted(ctx, StatusReasonUnexpectedShutdown)
}
