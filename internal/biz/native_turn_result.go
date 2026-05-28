package biz

// NativeTurnOutcome classifies native turn execution for Channel/Web callers.
//
// Deprecated: Use TurnOutcome instead.
type NativeTurnOutcome string

const (
	NativeTurnOutcomeCompleted NativeTurnOutcome = "completed"
	NativeTurnOutcomeQueued    NativeTurnOutcome = "queued"
	NativeTurnOutcomeRejected  NativeTurnOutcome = "rejected"
	NativeTurnOutcomeFailed    NativeTurnOutcome = "failed"
)

// NativeTurnResult is the explicit result of RunNativeTurnWithOutcome (P1).
//
// Deprecated: Use TurnResult instead.
type NativeTurnResult struct {
	Outcome      NativeTurnOutcome
	UserMsg      ChatMessage
	AssistantMsg ChatMessage
	PendingID    string
}
