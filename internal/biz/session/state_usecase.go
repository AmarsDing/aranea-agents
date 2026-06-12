package session

import (
	"context"
	"strings"

	"github.com/google/wire"
)

// SessionStateUsecase handles session state CRUD (key-value store).
// Extracted from SessionUsecase to reduce God Object scope.
// Stability:evolving
type SessionStateUsecase struct {
	stateRepo StateRepo
}

// NewSessionStateUsecase creates a new SessionStateUsecase.
func NewSessionStateUsecase(stateRepo StateRepo) *SessionStateUsecase {
	return &SessionStateUsecase{stateRepo: stateRepo}
}

func (uc *SessionStateUsecase) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	return uc.stateRepo.GetSessionState(ctx, sessionID)
}

func (uc *SessionStateUsecase) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return validationErr("session id is required")
	}
	return uc.stateRepo.SaveSessionState(ctx, sessionID, state)
}

func (uc *SessionStateUsecase) PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return validationErr("session id is required")
	}
	return uc.stateRepo.PatchSessionState(ctx, sessionID, sets, deletes)
}

func (uc *SessionStateUsecase) ApplyStateDelta(ctx context.Context, sessionID string, delta StateDelta) error {
	if delta.Path == "" {
		return nil
	}
	switch delta.Operation {
	case "set":
		return uc.stateRepo.PatchSessionState(ctx, sessionID, map[string]string{delta.Path: delta.ValueJSON}, nil)
	case "delete":
		return uc.stateRepo.PatchSessionState(ctx, sessionID, nil, []string{delta.Path})
	default:
		return uc.stateRepo.PatchSessionState(ctx, sessionID, map[string]string{delta.Path: delta.ValueJSON}, nil)
	}
}

// SessionStateProviderSet provides Wire bindings for SessionStateUsecase.
var SessionStateProviderSet = wire.NewSet(NewSessionStateUsecase)
