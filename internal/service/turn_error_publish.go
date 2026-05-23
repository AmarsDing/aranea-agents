package service

import (
	"context"
	"time"

	"aranea-agents/internal/event"
)

// publishTurnFailure emits a WS-visible error envelope for HTTP turn failures.
func (o *ChatOrchestrator) publishTurnFailure(sessionID, runID, source string, err error, pendingID string) {
	if o == nil || o.td.Pipeline.Bus == nil || err == nil {
		return
	}
	code := TurnErrorCodeFromErr(err)
	detail := ""
	if code == "" {
		detail = err.Error()
	}
	env := event.NewEnvelope(event.EnvelopeTypeError, source, sessionID)
	if runID != "" {
		env.InvocationID = runID
	}
	env.Error = envelopeErrorFromTurn(code, detail)
	if env.Error != nil && pendingID != "" {
		env.Error.PendingID = pendingID
	}
	publishCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	o.td.Pipeline.Bus.Publish(publishCtx, env)
}
