package session

import (
	"context"
	"strings"
)

func (uc *SessionUsecase) ListParticipants(ctx context.Context, sessionID string, repo SessionParticipantRepository) ([]SessionParticipant, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	sess, err := uc.sessions.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	messages, err := uc.listAllMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := repo.SyncFromSession(ctx, sess, messages); err != nil {
		return nil, err
	}
	return repo.ListBySession(ctx, sessionID)
}
