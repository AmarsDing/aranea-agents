package service

// publishTurnFailure emits a WS-visible error envelope for HTTP turn failures.
func (o *ChatOrchestrator) publishTurnFailure(sessionID, runID, source string, err error, pendingID string) {
	o.eventPublisher.PublishTurnFailure(sessionID, runID, source, err, pendingID)
}
