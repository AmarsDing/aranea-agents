package service

import "aranea-agents/pkg/apierror"

// publishTurnFailure emits a WS-visible error envelope for HTTP turn failures.
func (o *ChatOrchestrator) publishTurnFailure(sessionID, runID, source string, err error, pendingID string) {
	o.eventPublisher().PublishTurnFailure(sessionID, runID, source, err, pendingID)
}

// safeErrMsgForWS extracts a client-safe error message for WebSocket publishing.
// If err is an *apierror.Error, its Message is returned (already sanitized for
// Internal errors). Otherwise a generic "internal error" is returned to avoid
// leaking internal details (DB errors, provider state, etc.).
func safeErrMsgForWS(err error) string {
	if ae, ok := apierror.From(err); ok {
		return ae.Message
	}
	return "internal error"
}
