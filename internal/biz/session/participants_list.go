package session

import (
	"context"
	"strings"
)

func (uc *SessionUsecase) ListParticipants(ctx context.Context, sessionID string) ([]SessionParticipant, error) {
	if uc == nil || uc.participants == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	sess, err := uc.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	messages, err := uc.listAllMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := uc.participants.SyncFromSession(ctx, sess, messages); err != nil {
		return nil, err
	}
	return uc.participants.ListBySession(ctx, sessionID)
}
