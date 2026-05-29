package biz

import "context"

// TurnExecutorGateway is the narrow interface for turn execution operations.
// Consumers that only need to execute turns (e.g. WSServer) depend on this
// instead of the full TurnGateway.
type TurnExecutorGateway interface {
	ExecuteTurn(ctx context.Context, input TurnInput) (TurnResult, error)
	RunNativeTurn(ctx context.Context, input TurnInput) (ChatMessage, ChatMessage, error)
	RunNativeTurnWithOutcome(ctx context.Context, input TurnInput) (NativeTurnResult, error)
}

// TurnRunControlGateway is the narrow interface for run lifecycle control.
// Consumers that only need to check/cancel runs (e.g. DurableWorker) depend
// on this instead of the full TurnGateway.
type TurnRunControlGateway interface {
	HasActiveRun(sessionID string) bool
	CancelRun(ctx context.Context, sessionID string) bool
	SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string)
	LastPendingMessageID(sessionID string) string
}

// TurnGateway composes TurnExecutorGateway + TurnRunControlGateway for
// consumers that need both execution and run control (e.g. NativeTurnGateway).
type TurnGateway interface {
	TurnExecutorGateway
	TurnRunControlGateway
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
type DurableResumeGateway interface {
	ResumeDurableSessionRun(ctx context.Context, sessionRunID string) error
}

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
