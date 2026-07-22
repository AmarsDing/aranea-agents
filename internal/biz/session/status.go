package session

type SessionStatus string

const (
	SessionStatusIdle                 SessionStatus = "idle"
	SessionStatusRunning              SessionStatus = "running"
	SessionStatusCompleted            SessionStatus = "completed"
	SessionStatusInterrupted          SessionStatus = "interrupted"
	SessionStatusAwaitingConfirmation SessionStatus = "awaiting_confirmation"
)

type SessionStatusReason string

const (
	StatusReasonUserCancelled       SessionStatusReason = "user_cancelled"
	StatusReasonTimeout             SessionStatusReason = "timeout"
	StatusReasonUserEscalated       SessionStatusReason = "user_escalated"
	StatusReasonError               SessionStatusReason = "error"
	StatusReasonContextOverflow     SessionStatusReason = "context_overflow"
	StatusReasonServerShutdown      SessionStatusReason = "server_shutdown"
	StatusReasonUnexpectedShutdown  SessionStatusReason = "unexpected_shutdown"
	StatusReasonConfirmationTimeout SessionStatusReason = "confirmation_timeout"

	StatusReasonToolConfirmation   SessionStatusReason = "tool_confirmation"
	StatusReasonAgentAwaitingReply SessionStatusReason = "agent_awaiting_reply"
	StatusReasonClarification      SessionStatusReason = "clarification"

	StatusReasonManualOverride SessionStatusReason = "manual_override"
)

func IsProtectedStatus(s SessionStatus) bool {
	return s == SessionStatusRunning || s == SessionStatusAwaitingConfirmation
}
