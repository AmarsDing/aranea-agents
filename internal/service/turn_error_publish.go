package service

import (
	"context"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
)

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

// failTurn marks a fatal turn error and publishes the standard failure cascade:
// markTurnError → beforePublish (optional) → publishRunStatus(failed) →
// transitionSessionStatus(interrupted) → publishTurnFailure.
// Callers should return the result immediately after calling this.
//
// NOTE: Only use for BUILD-phase errors. EXECUTE/PERSIST-phase errors should
// only call markTurnError; the defer block in runSingleAgentViaTRPC handles
// the rest (publishRunStatus + transitionSessionStatus + publishTurnFailure).
//
// P1-03 fix: if the run was already cancelled (user clicked stop during BUILD),
// the "cancelled" status and session transition were already set by
// cancelActiveRun. We skip the "failed" publish and session transition to
// avoid conflicting terminal notifications. The cancel status is checked
// BEFORE beforePublish (which may call Finish and delete the status entry).
func (o *ChatOrchestrator) failTurn(
	ctx context.Context,
	sessionID, runID string,
	turnStatus *string, turnErr *error, turnErrMsg *string,
	err error,
	opts ...failTurnOption,
) (biz.ChatMessage, biz.ChatMessage, error) {
	opt := failTurnOpts{source: "chat-service"}
	for _, fn := range opts {
		fn(&opt)
	}
	markTurnError(turnStatus, turnErr, turnErrMsg, err)
	// P1-03 fix: check cancelled state BEFORE beforePublish (which may call
	// Finish and delete the run status entry).
	isCancelled := false
	if entry, ok := o.runs.GetStatus(sessionID); ok && entry.Status == biz.SessionRunPhaseCancelled {
		isCancelled = true
	}
	if opt.beforePublish != nil {
		opt.beforePublish()
	}
	if !isCancelled {
		o.publishRunStatus(sessionID, runID, "failed", safeErrMsgForWS(err))
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
	}
	o.publishTurnFailure(sessionID, runID, opt.source, err, "")
	return biz.ChatMessage{}, biz.ChatMessage{}, err
}

type failTurnOpts struct {
	source        string
	beforePublish func() // e.g., o.runs.Finish(sessionID) or emitter.LogWarn(...)
}
type failTurnOption func(*failTurnOpts)

// withBeforePublish registers a callback invoked after markTurnError but before
// publishRunStatus. Use it for cleanup (e.g., o.runs.Finish) or pre-publish
// logging (e.g., emitter.LogWarn) that must happen within the failure cascade.
func withBeforePublish(fn func()) failTurnOption {
	return func(o *failTurnOpts) { o.beforePublish = fn }
}

// markAndPublish records a turn error and publishes the failure event.
// Used in PERSIST-phase sub-operations (e.g., buildAndPersistAssistantMessage)
// where the full failTurn cascade is handled by the caller's defer block.
func (o *ChatOrchestrator) markAndPublish(
	sessionID, runID string,
	turnStatus *string, turnErr *error, turnErrMsg *string,
	err error,
) {
	markTurnError(turnStatus, turnErr, turnErrMsg, err)
	o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
}
