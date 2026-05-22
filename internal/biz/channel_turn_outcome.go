package biz

// ChannelTurnOutcome classifies the result of a Channel-bound native turn.
type ChannelTurnOutcome string

const (
	ChannelTurnOutcomeCompleted ChannelTurnOutcome = "completed"
	ChannelTurnOutcomeQueued    ChannelTurnOutcome = "queued"
	ChannelTurnOutcomeRejected  ChannelTurnOutcome = "rejected"
	ChannelTurnOutcomeFailed    ChannelTurnOutcome = "failed"
)

// ChannelTurnResult is the service-layer result of runChatTurnWithOutcome.
type ChannelTurnResult struct {
	Outcome   ChannelTurnOutcome
	Reply     string
	PendingID string
}
