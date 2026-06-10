package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

// sessionStateTransitor is the interface for session status transitions,
// extracted from ChatOrchestrator to isolate session lifecycle concerns.
type sessionStateTransitor interface {
	TransitionStatus(ctx context.Context, sessionID string, targetStatus sessstatus.SessionStatus, reason sessstatus.SessionStatusReason)
}

// chatSessionStateMgr implements sessionStateTransitor.
//
// Part of the TECH-DEBT(BL8) resolution: separating session state management
// from the orchestrator's core turn logic.
type chatSessionStateMgr struct {
	sessions biz.SessionTurnManager
	lg       loggateway.Logger
}

func newChatSessionStateMgr(sessions biz.SessionTurnManager, lg loggateway.Logger) *chatSessionStateMgr {
	return &chatSessionStateMgr{sessions: sessions, lg: lg}
}

// Compile-time interface check.
var _ sessionStateTransitor = (*chatSessionStateMgr)(nil)

// TransitionStatus transitions a session to the target status with the given reason.
// It is a no-op when the manager or session repo is nil, or when sessionID is blank.
func (m *chatSessionStateMgr) TransitionStatus(ctx context.Context, sessionID string, targetStatus sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) {
	if m == nil || m.sessions == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if err := m.sessions.TransitionStatus(ctx, sessionID, targetStatus, reason); err != nil {
		m.lg.Warn("session status transition failed",
			loggateway.StepID("chat.transition_status"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("target_status", string(targetStatus)),
			loggateway.Str("reason", string(reason)),
			loggateway.Err(err),
		)
	}
}
