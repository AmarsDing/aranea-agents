package session

import (
	"context"
	"strings"
)

type chatMessageStatusUpdater interface {
	UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error
}

// SearchMessages full-text search within one session.
func (uc *SessionUsecase) SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error) {
	if uc == nil || uc.sessions == nil {
		return MessageSearchResult{}, nil
	}
	if strings.TrimSpace(q.SessionID) == "" {
		return MessageSearchResult{}, validationErr("session_id is required")
	}
	if strings.TrimSpace(q.Keyword) == "" {
		return MessageSearchResult{}, validationErr("keyword is required")
	}
	return uc.sessions.SearchMessages(ctx, q)
}

func (uc *SessionUsecase) ListMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	res, err := uc.ListMessagesPaged(ctx, sessionID, 0, 0)
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// ListMessagesPaged returns messages with DB pagination (default limit when limit<=0).
func (uc *SessionUsecase) ListMessagesPaged(ctx context.Context, sessionID string, limit, offset int) (MessageListResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return MessageListResult{}, validationErr("session id is required")
	}
	if _, err := uc.sessions.GetSessionByID(ctx, sessionID); err != nil {
		return MessageListResult{}, err
	}
	total, err := uc.sessions.CountMessagesBySession(ctx, sessionID)
	if err != nil {
		return MessageListResult{}, err
	}
	limit = clampMessageListLimit(limit)
	if offset < 0 {
		offset = 0
	}
	items, err := uc.sessions.ListMessagesBySession(ctx, sessionID, limit, offset)
	if err != nil {
		return MessageListResult{}, err
	}
	return MessageListResult{Items: items, Total: total}, nil
}

// ListMessagesAfterTurn loads rows with turn_index > afterTurn (compression path).
func (uc *SessionUsecase) ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	return uc.sessions.ListMessagesAfterTurn(ctx, sessionID, afterTurn)
}

// ListMessagesByStatus loads recent rows matching status (e.g. tool_running cancel path).
func (uc *SessionUsecase) ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	return uc.sessions.ListMessagesByStatus(ctx, sessionID, status, limit)
}

// ListMessagesRecent loads the latest N messages in chronological order (timeline / cron).
func (uc *SessionUsecase) ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	return uc.sessions.ListMessagesRecent(ctx, sessionID, limit)
}

// AppendChatTurn persists a user + assistant pair (native chat).
func (uc *SessionUsecase) AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error {
	if err := uc.sessions.AppendChatTurn(ctx, sessionID, user, assistant); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(user.Role), "user") {
		_ = uc.maybeAutoTitleFromUserMessage(ctx, sessionID, user.ContentMarkdown)
	}
	return nil
}

// AppendChatMessage persists one chat row (streamed native turns).
func (uc *SessionUsecase) AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error {
	if err := uc.sessions.AppendChatMessage(ctx, sessionID, msg, bumpModelCall); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
		_ = uc.maybeAutoTitleFromUserMessage(ctx, sessionID, msg.ContentMarkdown)
	}
	return nil
}

func (uc *SessionUsecase) UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	status = strings.TrimSpace(status)
	if sessionID == "" || messageID == "" {
		return validationErr("session_id and message_id are required")
	}
	if status == "" {
		return validationErr("status is required")
	}
	updater, ok := uc.sessions.(chatMessageStatusUpdater)
	if !ok {
		return nil
	}
	return updater.UpdateChatMessageStatus(ctx, sessionID, messageID, status, strings.TrimSpace(errorMessage))
}

// UpdateMessageFeedback records thumbs up/down on an assistant message (options_json.feedback).
func (uc *SessionUsecase) UpdateMessageFeedback(ctx context.Context, sessionID, messageID, rating, comment string) error {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	rating = strings.TrimSpace(strings.ToLower(rating))
	if sessionID == "" || messageID == "" {
		return validationErr("session_id and message_id are required")
	}
	if rating != "positive" && rating != "negative" {
		return validationErr("rating must be positive or negative")
	}
	if _, err := uc.sessions.GetSessionByID(ctx, sessionID); err != nil {
		return err
	}
	return uc.sessions.UpdateMessageFeedbackJSON(ctx, sessionID, messageID, rating, strings.TrimSpace(comment))
}

// ListMessagesAfterRevision returns messages with turn_index > afterRevision*2 (M55 session sync).
func (uc *SessionUsecase) ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	if _, err := uc.sessions.GetSessionByID(ctx, sessionID); err != nil {
		return nil, err
	}
	return uc.sessions.ListMessagesAfterRevision(ctx, sessionID, afterRevision)
}

// BumpSessionRevision atomically increments session_revision after a completed turn.
func (uc *SessionUsecase) BumpSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, validationErr("session id is required")
	}
	return uc.sessions.BumpSessionRevision(ctx, sessionID)
}

// GetSessionRevision returns the current session_revision counter.
func (uc *SessionUsecase) GetSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, validationErr("session id is required")
	}
	return uc.sessions.GetSessionRevision(ctx, sessionID)
}

// UpsertChatActivityMessage persists a tool/MCP/Skill execution card for chat history restore.
func (uc *SessionUsecase) UpsertChatActivityMessage(ctx context.Context, sessionID string, msg ChatMessage) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return validationErr("session id is required")
	}
	if strings.TrimSpace(msg.ID) == "" {
		return validationErr("message id is required")
	}
	if _, err := uc.sessions.GetSessionByID(ctx, sessionID); err != nil {
		return err
	}
	return uc.sessions.UpsertChatActivityMessage(ctx, sessionID, msg)
}

// UpdateRunnerSnapshotJSON persists the Runner session snapshot.
func (uc *SessionUsecase) UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error {
	return uc.sessions.UpdateRunnerSnapshotJSON(ctx, sessionID, snapshotJSON)
}
