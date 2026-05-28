package biz

// ChannelTurnOutcome classifies the result of a Channel-bound native turn.
//
// Deprecated: Use TurnOutcome instead.
type ChannelTurnOutcome = TurnOutcome

const (
	// Deprecated: Use TurnOutcomeCompleted instead.
	ChannelTurnOutcomeCompleted TurnOutcome = "completed"
	// Deprecated: Use TurnOutcomeQueued instead.
	ChannelTurnOutcomeQueued TurnOutcome = "queued"
	// Deprecated: Use TurnOutcomeRejected instead.
	ChannelTurnOutcomeRejected TurnOutcome = "rejected"
	// Deprecated: Use TurnOutcomeFailed instead.
	ChannelTurnOutcomeFailed TurnOutcome = "failed"
)

// ChannelTurnResult is the service-layer result of runChatTurnWithOutcome.
//
// Deprecated: Use TurnResult instead.
type ChannelTurnResult struct {
	Outcome   TurnOutcome
	Reply     string
	PendingID string
}
