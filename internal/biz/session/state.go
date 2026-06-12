package session

import "context"

// GetSessionState delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	return uc.messageUsecase.GetSessionState(ctx, sessionID)
}

// SaveSessionState delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	return uc.messageUsecase.SaveSessionState(ctx, sessionID, state)
}

// PatchSessionState delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error {
	return uc.messageUsecase.PatchSessionState(ctx, sessionID, sets, deletes)
}

// ApplyStateDelta delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ApplyStateDelta(ctx context.Context, sessionID string, delta StateDelta) error {
	return uc.messageUsecase.ApplyStateDelta(ctx, sessionID, delta)
}
