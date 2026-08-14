package biz

import "context"

// TurnExecutorGateway is the narrow interface for turn execution operations.
// Consumers that only need to execute turns (e.g. WSServer) depend on this
// instead of the full TurnGateway.
// Stability:evolving
type TurnExecutorGateway interface {
	ExecuteTurn(ctx context.Context, input TurnInput) (TurnResult, error)
	RunNativeTurn(ctx context.Context, input TurnInput) (ChatMessage, ChatMessage, error)
	RunNativeTurnWithOutcome(ctx context.Context, input TurnInput) (TurnResult, error)
}

// TurnRunControlGateway is the narrow interface for run lifecycle control.
// Consumers that only need to check/cancel runs (e.g. DurableWorker) depend
// on this instead of the full TurnGateway.
// Stability:evolving
type TurnRunControlGateway interface {
	HasActiveRun(sessionID string) bool
	CancelRun(ctx context.Context, sessionID string) bool
	SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string)
	LastPendingMessageID(sessionID string) string
}

// TurnGateway composes TurnExecutorGateway + TurnRunControlGateway for
// consumers that need both execution and run control (e.g. ChannelTurnGateway).
// Stability:evolving
type TurnGateway interface {
	TurnExecutorGateway
	TurnRunControlGateway
}

// TurnControlGateway extends TurnGateway with run control operations needed
// by Channel card actions and session run management.
// Stability:evolving
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

	// ConfirmToolGateForCard resolves a tool-blocked confirm step from a channel
	// card action. replyToken 为 serviceawaitreply 结构化 token；归属校验由调用方
	// 的渠道 peer 绑定完成，此处仅校验 step 归属与状态机。
	ConfirmToolGateForCard(ctx context.Context, sessionID, stepID, replyToken string) (accepted bool, reply string)

	// SubmitClarificationForCard submits clarification answers from a channel
	// card action. 语义同 SubmitClarification RPC（无 ctxuser 校验）。
	SubmitClarificationForCard(ctx context.Context, sessionID, stepID string, answers []ClarificationAnswer) (reply string, err error)
}

// PendingMessageGateway is the narrow interface for pending message operations.
// Split from ChatService so that consumers only depend on what they need.
// Stability:evolving
type DurableResumeGateway interface {
	ResumeDurableSessionRun(ctx context.Context, sessionRunID string) error
}

// Stability:evolving
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
