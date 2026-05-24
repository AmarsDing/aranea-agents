package biz

// NativeTurnOutcome classifies native turn execution for Channel/Web callers.
type NativeTurnOutcome string

const (
	NativeTurnOutcomeCompleted NativeTurnOutcome = "completed"
	NativeTurnOutcomeQueued    NativeTurnOutcome = "queued"
	NativeTurnOutcomeFailed    NativeTurnOutcome = "failed"
)

// NativeTurnResult is the explicit result of RunNativeTurnWithOutcome (P1).
type NativeTurnResult struct {
	Outcome      NativeTurnOutcome
	UserMsg      ChatMessage
	AssistantMsg ChatMessage
	PendingID    string
}
