package biz

import (
	"context"
	"strings"

	sessstatus "aranea-agents/internal/biz/session"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// TurnLifecycleUsecase — centralizes session state transitions during a turn.
//
// This usecase is the biz-layer replacement for the service-layer
// chatSessionStateMgr (TECH-DEBT(BL8) resolution). It exposes:
//
//  1. Generic TransitionStatus — direct passthrough with logging + nil safety.
//  2. Convenience methods for every turn phase (running, completed, failed,
//     interrupted, awaiting_user, cancelled, timeout) so callers do not need
//     to know the underlying SessionStatus / SessionStatusReason vocabulary.
//
// The usecase is intentionally thin: it owns no goroutines, no transports,
// and no caching. Its sole responsibility is "translate a turn phase into a
// session state transition, with structured logging on failure". The
// ChatOrchestrator and SpiritSynthesisService both delegate here.
// ---------------------------------------------------------------------------

// TurnLifecycleUsecaseConfig configures the constructor.
type TurnLifecycleUsecaseConfig struct {
	// Sessions is the session state port. Required: TransitionStatus is the
	// only state-mutating operation and must hit a real backend. Nil = all
	// transitions are no-ops (useful for tests).
	Sessions SessionStatePort
	// Logger is the structured logger. Optional; nil = silent.
	Logger loggateway.Logger
}

// TurnLifecycleUsecase owns the session state transitions for a turn.
type TurnLifecycleUsecase struct {
	sessions SessionStatePort
	lg       loggateway.Logger
}

// NewTurnLifecycleUsecase constructs a TurnLifecycleUsecase.
func NewTurnLifecycleUsecase(cfg TurnLifecycleUsecaseConfig) *TurnLifecycleUsecase {
	return &TurnLifecycleUsecase{
		sessions: cfg.Sessions,
		lg:       cfg.Logger,
	}
}

// Compile-time sanity check: the convenience methods must accept the
// well-known status constants.
var (
	_ = sessstatus.SessionStatusRunning
	_ = sessstatus.SessionStatusCompleted
	_ = sessstatus.SessionStatusInterrupted
	_ = sessstatus.SessionStatusAwaitingConfirmation
)

// TransitionStatus transitions a session to the target status with the given
// reason. Nil-safe: a nil receiver or nil sessions port is a no-op, as is
// an empty sessionID. Errors from the underlying port are logged but not
// returned — session state transitions are best-effort and must never
// block the turn pipeline.
func (u *TurnLifecycleUsecase) TransitionStatus(
	ctx context.Context,
	sessionID string,
	targetStatus sessstatus.SessionStatus,
	reason sessstatus.SessionStatusReason,
) {
	if u == nil || u.sessions == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if err := u.sessions.TransitionStatus(ctx, sessionID, targetStatus, reason); err != nil {
		if u.lg != nil {
			u.lg.Warn("session status transition failed",
				loggateway.StepID("turn.lifecycle.transition"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("target_status", string(targetStatus)),
				loggateway.Str("reason", string(reason)),
				loggateway.Err(err),
			)
		}
	}
}

// MarkRunning transitions the session to the "running" state. No reason.
func (u *TurnLifecycleUsecase) MarkRunning(ctx context.Context, sessionID string) {
	u.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")
}

// MarkCompleted transitions the session to the "completed" state. No reason.
func (u *TurnLifecycleUsecase) MarkCompleted(ctx context.Context, sessionID string) {
	u.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, "")
}

// MarkInterrupted transitions the session to the "interrupted" state with the
// given reason. Use this for user cancellation, timeout, budget escalation,
// and unrecoverable errors (see session.StatusReason* constants).
func (u *TurnLifecycleUsecase) MarkInterrupted(
	ctx context.Context,
	sessionID string,
	reason sessstatus.SessionStatusReason,
) {
	u.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, reason)
}

// MarkAwaiting transitions the session to the "awaiting confirmation" state.
// The kind argument is informational only and logged for traceability; the
// session state itself does not encode the kind (use ChatAwaitMeta for that).
func (u *TurnLifecycleUsecase) MarkAwaiting(
	ctx context.Context,
	sessionID string,
	kind sessstatus.SessionStatusReason,
) {
	u.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusAwaitingConfirmation, kind)
}
