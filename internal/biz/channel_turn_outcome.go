package biz

// ChannelTurnResult is the service-layer result of runChatTurnWithOutcome.
// Unlike TurnResult it carries PendingID for the queued outcome.
// Stability:evolving
type ChannelTurnResult struct {
	Outcome   TurnOutcome
	Reply     string
	PendingID string
}
