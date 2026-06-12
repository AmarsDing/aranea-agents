package session

import "context"

// CreateTurn delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) CreateTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error) {
	return uc.messageUsecase.CreateTurn(ctx, turn)
}

// UpdateTurn delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error) {
	return uc.messageUsecase.UpdateTurn(ctx, id, fields)
}

// IncrementInvocationCounts delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error {
	return uc.messageUsecase.IncrementInvocationCounts(ctx, sessionID, toolDelta, mcpDelta, skillDelta)
}

// ListTurns delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ListTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error) {
	return uc.messageUsecase.ListTurns(ctx, sessionID, limit, offset)
}
