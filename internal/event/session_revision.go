package event

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// SessionRevisionBumper increments sessions.session_revision.
type SessionRevisionBumper interface {
	BumpSessionRevision(ctx context.Context, sessionID string) (int64, error)
}

// BumpSessionRevision bumps the session revision counter after a turn or
// message persist. Errors are logged but not returned to avoid blocking the
// turn flow.
//
// ADR-03 Phase 5 Blocker D: the legacy publish half (PublishSessionRevision
// envelope to SessionBus) has been removed — SessionBus had no live
// subscriber (TurnPreviewCoordinator.consume was a no-op after Phase 1c-5).
// The bump half remains effective: it increments sessions.session_revision
// in the DB, which the frontend reads via ListActivities / GetSession RPCs.
func BumpSessionRevision(ctx context.Context, bumper SessionRevisionBumper, sessionID string, lg loggateway.Logger) {
	if bumper == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if _, err := bumper.BumpSessionRevision(ctx, sessionID); err != nil {
		if lg != nil {
			lg.Warn("session_revision bump failed",
				loggateway.StepID("session.revision.bump"),
				loggateway.Str("session_id", sessionID),
				loggateway.Err(err))
		}
	}
}
