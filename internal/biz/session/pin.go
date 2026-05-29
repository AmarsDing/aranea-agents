package session

import (
	"context"
	"strings"
)

func (uc *SessionUsecase) Pin(ctx context.Context, id string) (Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, validationErr("session id is required")
	}
	return uc.sessionPinWriter.PinSession(ctx, id)
}

func (uc *SessionUsecase) Unpin(ctx context.Context, id string) (Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, validationErr("session id is required")
	}
	return uc.sessionPinWriter.UnpinSession(ctx, id)
}
