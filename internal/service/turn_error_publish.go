package service

import (
	"context"
	"errors"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
)

// isUserCancelledErr 是三处团队回调（executeTeamTurnViaHooks / pending-queue
// / StartTeamTurn）共用的「用户取消」判别（87-orchestration-chain-hardening
// D1-F1）：err 链 context.Canceled / ctx.Err() / context.Cause(ctx) 三源
// 任一命中即视为用户取消。显式不把 context.DeadlineExceeded 判为取消——
// TeamMaxLifetime 墙钟超时是 infra 失败语义，必须走 failed 路径。
func isUserCancelledErr(ctx context.Context, err error) bool {
	if err != nil && errors.Is(err, context.Canceled) {
		return true
	}
	if ctx == nil {
		return false
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return true
	}
	if cause := context.Cause(ctx); cause != nil && errors.Is(cause, context.Canceled) {
		return true
	}
	return false
}

// runWasCancelled reports whether the run should be treated as user-cancelled
// rather than failed (C-10). Checks in-memory status first, then context /
// turn error cancellation signals.
func (o *ChatOrchestrator) runWasCancelled(ctx context.Context, sessionID string, turnErr error) bool {
	if o != nil && o.runs != nil {
		if entry, ok := o.runs.GetStatus(sessionID); ok && entry.Status == biz.SessionRunPhaseCancelled {
			return true
		}
	}
	// D1-F1：取消信号判别委托给包级 isUserCancelledErr，TeamStarter 侧
	// （spirit_team.go StartTeamTurn）共享同一实现，三处回调分流口径一致。
	return isUserCancelledErr(ctx, turnErr)
}

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
	// C-10 / P1-03: check cancelled BEFORE beforePublish (which may call
	// Finish and delete the run status entry).
	isCancelled := o.runWasCancelled(ctx, sessionID, err)
	if opt.beforePublish != nil {
		opt.beforePublish()
	}
	if !isCancelled {
		// C-11: persist terminal status before (and with) WS publish — do not
		// use publishRunStatus alone, which skips PersistRunStatus.
		failCtx := ctx
		if failCtx == nil || failCtx.Err() != nil {
			failCtx = context.Background()
		}
		o.setRunStatus(failCtx, sessionID, runID, "failed", safeErrMsgForWS(err))
		o.transitionSessionStatus(failCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
	}
	o.publishTurnFailure(sessionID, runID, opt.source, err, "")
	return biz.ChatMessage{}, biz.ChatMessage{}, err
}

type failTurnOpts struct {
	source        string
	beforePublish func() // e.g., o.runs.Finish(sessionID, runID) or emitter.LogWarn(...)
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
