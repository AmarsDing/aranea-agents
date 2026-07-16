package session

import (
	"context"
)

// NoopMessageWriter is a no-op MessageWriter implementation for Phase 1c-3.
// All write operations are no-ops because the ActivityProjector now handles
// persistence to the activities table. The messages table has been removed.
type NoopMessageWriter struct{}

// NewNoopMessageWriter creates a new NoopMessageWriter.
func NewNoopMessageWriter() *NoopMessageWriter {
	return &NoopMessageWriter{}
}

// AppendChatTurn is a no-op; Activities are persisted by ActivityProjector.
func (w *NoopMessageWriter) AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error {
	return nil
}

// AppendChatMessage is a no-op; Activities are persisted by ActivityProjector.
func (w *NoopMessageWriter) AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error {
	return nil
}

// UpdateMessageFeedbackJSON is a no-op; feedback is not yet migrated to Activities.
// TODO(phase-3): Store feedback in Activity.Meta when needed.
func (w *NoopMessageWriter) UpdateMessageFeedbackJSON(ctx context.Context, sessionID, messageID, rating, comment string) error {
	return nil
}

// UpsertChatActivityMessage is a no-op; Activities are persisted by ActivityProjector.
func (w *NoopMessageWriter) UpsertChatActivityMessage(ctx context.Context, sessionID string, msg ChatMessage) (bool, error) {
	return false, nil
}

// NoopMessageStatusWriter is a no-op MessageStatusWriter.
type NoopMessageStatusWriter struct{}

// NewNoopMessageStatusWriter creates a new NoopMessageStatusWriter.
func NewNoopMessageStatusWriter() *NoopMessageStatusWriter {
	return &NoopMessageStatusWriter{}
}

// UpdateChatMessageStatus is a no-op; Activity status is managed by ActivityProjector.
func (w *NoopMessageStatusWriter) UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error {
	return nil
}

// noopActivityLister returns empty results for all Activity list calls.
// Used as a fallback when no ActivityLister is wired (tests/CLI).
type noopActivityLister struct{}

func (noopActivityLister) ListBySessionTurn(ctx context.Context, sessionID, turnID string) ([]ActivityEntry, error) {
	return nil, nil
}

func (noopActivityLister) ListBySession(ctx context.Context, sessionID string) ([]ActivityEntry, error) {
	return nil, nil
}

// Compile-time interface checks.
var (
	_ MessageReader       = (*ActivityMessageReader)(nil)
	_ MessageSearchReader = (*ActivityMessageReader)(nil)
	_ MessageWriter       = (*NoopMessageWriter)(nil)
	_ MessageStatusWriter = (*NoopMessageStatusWriter)(nil)
	_ ActivityLister      = noopActivityLister{}
)
