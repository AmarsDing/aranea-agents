package event

import (
	"context"
	"strings"
)

// Session run_status values for revision envelopes (M55).
const (
	SessionRunStatusSync      = "sync"      // mid-turn message persist; not turn complete
	SessionRunStatusCompleted = "completed" // turn successfully closed
)

// SessionRevisionBumper increments sessions.session_revision.
type SessionRevisionBumper interface {
	BumpSessionRevision(ctx context.Context, sessionID string) (int64, error)
}

// NotifySessionRevisionSync publishes the current revision with status=sync (no bump).
func NotifySessionRevisionSync(ctx context.Context, sessions SessionRevisionReader, bus Bus, sessionID, runID, turnID, source string) {
	if sessions == nil || bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	rev, err := sessions.GetSessionRevision(ctx, sessionID)
	if err != nil || rev <= 0 {
		return
	}
	PublishSessionRevisionEnvelope(bus, sessionID, runID, turnID, source, rev, SessionRunStatusSync)
}

// SessionRevisionReader reads the current session revision counter.
type SessionRevisionReader interface {
	GetSessionRevision(ctx context.Context, sessionID string) (int64, error)
}

// BumpAndPublishSessionRevision bumps revision after a completed turn (status=completed).
func BumpAndPublishSessionRevision(ctx context.Context, bumper SessionRevisionBumper, bus Bus, sessionID, runID, turnID, source string) {
	bumpAndPublishSessionRevision(ctx, bumper, bus, sessionID, runID, turnID, source, SessionRunStatusCompleted)
}

// BumpAndPublishSessionRevisionSync bumps revision after user message persist (status=sync).
// Web clients must hydrate incrementally without treating this as turn completion.
func BumpAndPublishSessionRevisionSync(ctx context.Context, bumper SessionRevisionBumper, bus Bus, sessionID, runID, turnID, source string) {
	bumpAndPublishSessionRevision(ctx, bumper, bus, sessionID, runID, turnID, source, SessionRunStatusSync)
}

func bumpAndPublishSessionRevision(ctx context.Context, bumper SessionRevisionBumper, bus Bus, sessionID, runID, turnID, source, status string) {
	if bumper == nil || bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	rev, err := bumper.BumpSessionRevision(ctx, sessionID)
	if err != nil {
		CtxFlowLogWarn(ctx, "session.revision.bump", "session_revision bump failed",
			P("session_id", sessionID), P("error", err.Error()))
		return
	}
	PublishSessionRevisionEnvelope(bus, sessionID, runID, turnID, source, rev, status)
}

// PublishSessionRevisionEnvelope emits run_status with session_revision for Web incremental sync.
func PublishSessionRevisionEnvelope(bus Bus, sessionID, runID, turnID, source string, revision int64, status string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" || revision <= 0 {
		return
	}
	if strings.TrimSpace(status) == "" {
		status = SessionRunStatusCompleted
	}
	env := NewEnvelope(EnvelopeTypeRunStatus, "session-sync", sessionID)
	env.Channel = RouteChannel(env)
	env.SessionRevision = revision
	env.TurnID = strings.TrimSpace(turnID)
	src := strings.TrimSpace(source)
	if src != "" {
		env.Source = src
	}
	env.Metadata = map[string]any{
		"run_id":           runID,
		"status":           status,
		"session_revision": revision,
	}
	if src == "ws" && status == SessionRunStatusSync {
		env.Metadata["skip_hydrate"] = true
	}
	bus.Publish(context.Background(), env)
}
