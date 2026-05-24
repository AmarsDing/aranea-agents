package biz

import "context"

// SessionProjection is the biz-level port for reading session state projections.
// Consumers (Channel, Monitor, Team observer) depend on this instead of
// reaching into SessionUsecase or ChatService internals.
//
// Implementations live in internal/service or internal/biz/session.
// Wire binding happens in internal/service.
type SessionProjection interface {
	// GetMessages returns messages for a session after the given revision.
	GetMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int) ([]ChatMessage, error)

	// GetLatestRevision returns the latest message revision for a session.
	GetLatestRevision(ctx context.Context, sessionID string) (int, error)

	// GetSessionActivity returns the current activity state for a session.
	GetSessionActivity(ctx context.Context, sessionID string) (*SessionActivity, error)
}

// SessionActivity is a read model summarizing the current state of a session
// for projection consumers.
type SessionActivity struct {
	SessionID   string
	RunStatus   string
	RunID       string
	HasActive   bool
	PendingIDs  []string
	AwaitKind   string
	AwaitToolKey string
}
