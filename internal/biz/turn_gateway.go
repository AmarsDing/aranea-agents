package biz

import "context"

// TurnGateway is the narrow interface for turn lifecycle operations that
// consumers (Channel ingress, Cron runner, A2A endpoint, WS handler) need.
// It splits the monolithic ChatService into focused interfaces so that
// consumers only depend on what they actually use.
//
// Implementations live in internal/service (ChatOrchestrator implements this).
// Wire binding happens in internal/service.
type TurnGateway interface {
	// ExecuteTurn runs a single turn and returns the classified result.
	ExecuteTurn(ctx context.Context, input TurnInput) (TurnResult, error)

	// HasActiveRun reports whether a session has an in-flight run.
	HasActiveRun(sessionID string) bool

	// CancelRun stops the active run for a session.
	CancelRun(ctx context.Context, sessionID string) bool

	// SetRunStatus atomically updates the run status and publishes a WS envelope.
	SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string)

	// LastPendingMessageID returns the most recently enqueued pending message ID.
	LastPendingMessageID(sessionID string) string
}

// TurnControlGateway extends TurnGateway with run control operations needed
// by Channel card actions and session run management.
type TurnControlGateway interface {
	TurnGateway

	// CancelSessionRunForCard cancels a session run by ID for card action callbacks.
	CancelSessionRunForCard(ctx context.Context, sessionRunID, expectedSessionID string) (cancelled bool, reply string)

	// ActiveSessionRunPhase returns the phase of the active session run, if any.
	ActiveSessionRunPhase(ctx context.Context, sessionID string) string

	// EscalateActiveSessionRun escalates the active session run to background.
	EscalateActiveSessionRun(ctx context.Context, sessionID string) (escalated bool, reply string, err error)

	// EscalateSessionRun escalates a specific session run to background.
	EscalateSessionRun(ctx context.Context, sessionRunID, expectedSessionID string) (reply string, err error)
}

// PendingMessageGateway is the narrow interface for pending message operations.
// Split from ChatService so that consumers only depend on what they need.
type PendingMessageGateway interface {
	// EnqueueUserMessage adds a user message to the pending queue.
	EnqueueUserMessage(ctx context.Context, sessionID, content string) (accepted bool, pendingID string, rejectReason string, err error)

	// CancelPendingMessage cancels a pending message.
	CancelPendingMessage(ctx context.Context, sessionID, pendingID string) error

	// UpdatePendingMessage updates the content of a pending message.
	UpdatePendingMessage(ctx context.Context, sessionID, pendingID, content string) error

	// GetPendingMessages returns pending messages for a session.
	GetPendingMessages(ctx context.Context, sessionID string) ([]PendingMessage, error)
}

// PendingMessage is a simplified view of a pending user message.
type PendingMessage struct {
	ID        string
	SessionID string
	Content   string
	CreatedAt string
}
