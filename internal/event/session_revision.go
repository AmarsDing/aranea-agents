package event

import (
	"context"
	"strings"
)

// SessionRevisionBumper increments sessions.session_revision after a completed turn.
type SessionRevisionBumper interface {
	BumpSessionRevision(ctx context.Context, sessionID string) (int64, error)
}

// BumpAndPublishSessionRevision bumps revision and emits a session-sync envelope.
func BumpAndPublishSessionRevision(ctx context.Context, bumper SessionRevisionBumper, bus Bus, sessionID, runID, turnID, source string) {
	if bumper == nil || bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	rev, err := bumper.BumpSessionRevision(ctx, sessionID)
	if err != nil {
		CtxFlowLogWarn(ctx, "session.revision.bump", "session_revision bump failed",
			P("session_id", sessionID), P("error", err.Error()))
		return
	}
	PublishSessionRevisionEnvelope(bus, sessionID, runID, turnID, source, rev)
}

// PublishSessionRevisionEnvelope emits run_status with session_revision for Web incremental sync.
func PublishSessionRevisionEnvelope(bus Bus, sessionID, runID, turnID, source string, revision int64) {
	if bus == nil || strings.TrimSpace(sessionID) == "" || revision <= 0 {
		return
	}
	env := NewEnvelope(EnvelopeTypeRunStatus, "session-sync", sessionID)
	env.Channel = RouteChannel(env)
	env.SessionRevision = revision
	env.TurnID = strings.TrimSpace(turnID)
	if src := strings.TrimSpace(source); src != "" {
		env.Source = src
	}
	env.Metadata = map[string]any{
		"run_id":           runID,
		"status":           "completed",
		"session_revision": revision,
	}
	bus.Publish(context.Background(), env)
}
