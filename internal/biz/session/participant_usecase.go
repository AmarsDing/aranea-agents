package session

import (
	"context"
	"strings"

	"github.com/google/wire"
)

// SessionParticipantUsecase handles session participant listing.
// Extracted from SessionUsecase to reduce God Object scope.
// Stability:evolving
type SessionParticipantUsecase struct {
	participants  SessionParticipantRepository
	sessionReader SessionReader
	messageReader MessageReader
}

// NewSessionParticipantUsecase creates a new SessionParticipantUsecase.
func NewSessionParticipantUsecase(participants SessionParticipantRepository, sessionReader SessionReader, messageReader MessageReader) *SessionParticipantUsecase {
	return &SessionParticipantUsecase{
		participants:  participants,
		sessionReader: sessionReader,
		messageReader: messageReader,
	}
}

func (uc *SessionParticipantUsecase) ListParticipants(ctx context.Context, sessionID string) ([]SessionParticipant, error) {
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

// listAllMessages loads all messages for a session (used by participant sync).
func (uc *SessionParticipantUsecase) listAllMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	total, err := uc.messageReader.CountMessagesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	out := make([]ChatMessage, 0, total)
	for offset := 0; offset < total; {
		limit := MessageListMaxLimit
		if remaining := total - offset; remaining < limit {
			limit = remaining
		}
		chunk, err := uc.messageReader.ListMessagesBySession(ctx, sessionID, limit, offset)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
		offset += len(chunk)
		if len(chunk) == 0 {
			break
		}
	}
	return out, nil
}

// SessionParticipantProviderSet provides Wire bindings for SessionParticipantUsecase.
var SessionParticipantProviderSet = wire.NewSet(NewSessionParticipantUsecase)
