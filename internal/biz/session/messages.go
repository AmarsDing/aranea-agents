package session

import "context"

// Stability:stable
type MessageStatusWriter interface {
	UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error
}

// SearchMessages delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error) {
	if uc == nil || uc.messageUsecase == nil {
		return MessageSearchResult{}, nil
	}
	return uc.messageUsecase.SearchMessages(ctx, q)
}

// ListMessages delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ListMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	return uc.messageUsecase.ListMessages(ctx, sessionID)
}

// ListMessagesPaged delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ListMessagesPaged(ctx context.Context, sessionID string, limit, offset int) (MessageListResult, error) {
	return uc.messageUsecase.ListMessagesPaged(ctx, sessionID, limit, offset)
}

// ListMessagesAfterTurn delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error) {
	return uc.messageUsecase.ListMessagesAfterTurn(ctx, sessionID, afterTurn)
}

// ListMessagesByStatus delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]ChatMessage, error) {
	return uc.messageUsecase.ListMessagesByStatus(ctx, sessionID, status, limit)
}

// ListMessagesRecent delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	return uc.messageUsecase.ListMessagesRecent(ctx, sessionID, limit)
}

// AppendChatTurn delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error {
	return uc.messageUsecase.AppendChatTurn(ctx, sessionID, user, assistant)
}

// AppendChatMessage delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error {
	return uc.messageUsecase.AppendChatMessage(ctx, sessionID, msg, bumpModelCall)
}

// UpdateChatMessageStatus delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error {
	return uc.messageUsecase.UpdateChatMessageStatus(ctx, sessionID, messageID, status, errorMessage)
}

// UpdateMessageFeedback delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateMessageFeedback(ctx context.Context, sessionID, messageID, rating, comment string) error {
	return uc.messageUsecase.UpdateMessageFeedback(ctx, sessionID, messageID, rating, comment)
}

// ListMessagesAfterRevision delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error) {
	return uc.messageUsecase.ListMessagesAfterRevision(ctx, sessionID, afterRevision)
}

// BumpSessionRevision delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) BumpSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	return uc.messageUsecase.BumpSessionRevision(ctx, sessionID)
}

// GetSessionRevision delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) GetSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	return uc.messageUsecase.GetSessionRevision(ctx, sessionID)
}

// UpsertChatActivityMessage delegates to SessionMessageUsecase (Facade pattern).
func (uc *SessionUsecase) UpsertChatActivityMessage(ctx context.Context, sessionID string, msg ChatMessage) error {
	return uc.messageUsecase.UpsertChatActivityMessage(ctx, sessionID, msg)
}

// UpdateRunnerSnapshotJSON persists the Runner session snapshot.
func (uc *SessionUsecase) UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error {
	return uc.compressionUsecase.UpdateRunnerSnapshotJSON(ctx, sessionID, snapshotJSON)
}
