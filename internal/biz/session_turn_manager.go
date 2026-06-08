package biz

import (
	"context"

	"aranea-agents/internal/biz/session"
)

// ---------------------------------------------------------------------------
// SessionTurnManager — the interface used by rt.TurnDeps.Sessions.
//
// Replaces the previous *SessionUsecase concrete type in TurnDeps, eliminating
// the type assertion in team/runner.go. Composed of four narrow sub-interfaces,
// each covering a single responsibility domain (≤5 methods).
// ---------------------------------------------------------------------------

// SessionCRUDPort covers basic session CRUD operations needed during a turn.
type SessionCRUDPort interface {
	Get(ctx context.Context, id string) (session.Session, error)
	Create(ctx context.Context, in session.Session) (session.Session, error)
	Update(ctx context.Context, id string, fields session.SessionUpdateFields) (session.Session, error)
}

// SessionTurnWriterPort covers turn and message mutation operations during a turn.
type SessionTurnWriterPort interface {
	AppendChatMessage(ctx context.Context, sessionID string, msg session.ChatMessage, bumpModelCall bool) error
	UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error
	CreateTurn(ctx context.Context, turn session.SessionTurn) (session.SessionTurn, error)
	UpdateTurn(ctx context.Context, id string, fields session.SessionTurnUpdateFields) (session.SessionTurn, error)
}

// SessionStatePort covers session state and revision operations during a turn.
type SessionStatePort interface {
	GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
	SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
	PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error
	GetSessionRevision(ctx context.Context, sessionID string) (int64, error)
	BumpSessionRevision(ctx context.Context, sessionID string) (int64, error)
	TransitionStatus(ctx context.Context, sessionID string, target session.SessionStatus, reason session.SessionStatusReason) error
}

// SessionTurnExtrasPort covers auxiliary turn operations (feedback, metrics, state delta, activity messages).
type SessionTurnExtrasPort interface {
	UpdateMessageFeedback(ctx context.Context, sessionID, messageID, rating, comment string) error
	IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error
	UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error
	UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
	ApplyStateDelta(ctx context.Context, sessionID string, delta session.StateDelta) error
	AccumulateMetricsDelta(delta session.SessionMetricsDelta)
	ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]session.ChatMessage, error)
	UpsertChatActivityMessage(ctx context.Context, sessionID string, msg session.ChatMessage) error
}

// SessionTurnManager composes all four session sub-interfaces needed by the
// turn execution infrastructure (ChatOrchestrator, Team Runner, event consumers).
// Replaces the previous *SessionUsecase concrete dependency in rt.TurnDeps.
type SessionTurnManager interface {
	SessionCRUDPort
	SessionTurnWriterPort
	SessionStatePort
	SessionTurnExtrasPort
}

// ---------------------------------------------------------------------------
// Compile-time assertions that SessionUsecase satisfies the sub-interfaces.
// ---------------------------------------------------------------------------

var _ SessionCRUDPort = (*SessionUsecase)(nil)
var _ SessionTurnWriterPort = (*SessionUsecase)(nil)
var _ SessionStatePort = (*SessionUsecase)(nil)
var _ SessionTurnExtrasPort = (*SessionUsecase)(nil)
var _ SessionTurnManager = (*SessionUsecase)(nil)
