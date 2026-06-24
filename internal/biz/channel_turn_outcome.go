package biz

// ChannelTurnOutcome classifies the result of a Channel-bound native turn.
//
// Deprecated: Use TurnOutcome instead.
type ChannelTurnOutcome = TurnOutcome

// ChannelTurnResult is the service-layer result of runChatTurnWithOutcome.
//
// Deprecated: Use TurnResult instead.
type ChannelTurnResult struct {
	Outcome   TurnOutcome
	Reply     string
	PendingID string
}
