package session

import (
	"context"
	"strings"
)

func (uc *SessionUsecase) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	return uc.stateRepo.GetSessionState(ctx, sessionID)
}

func (uc *SessionUsecase) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return validationErr("session id is required")
	}
	return uc.stateRepo.SaveSessionState(ctx, sessionID, state)
}

func (uc *SessionUsecase) ApplyStateDelta(ctx context.Context, sessionID string, delta StateDelta) error {
	if delta.Path == "" {
		return nil
	}
	state, err := uc.stateRepo.GetSessionState(ctx, sessionID)
	if err != nil {
		return err
	}
	switch delta.Operation {
	case "set":
		state[delta.Path] = delta.ValueJSON
	case "append":
		existing, _ := state[delta.Path]
		state[delta.Path] = existing + delta.ValueJSON
	case "delete":
		delete(state, delta.Path)
	default:
		state[delta.Path] = delta.ValueJSON
	}
	return uc.stateRepo.SaveSessionState(ctx, sessionID, state)
}
