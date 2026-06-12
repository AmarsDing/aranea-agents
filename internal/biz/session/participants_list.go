package session

import "context"

// ListParticipants delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ListParticipants(ctx context.Context, sessionID string) ([]SessionParticipant, error) {
	if uc == nil || uc.messageUsecase == nil {
		return nil, nil
	}
	return uc.messageUsecase.ListParticipants(ctx, sessionID)
}
