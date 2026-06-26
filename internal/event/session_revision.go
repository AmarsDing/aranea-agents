package event

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// Session run_status values for revision envelopes (M55).
const (
	SessionRunStatusSync      = "sync"      // mid-turn message persist; not turn complete
	SessionRunStatusCompleted = "completed" // turn successfully closed
)

// TECH-DEBT(ADR-03 Phase 5 Tier 5 / Blocker D): session-revision Envelope
// publish path is a DEAD LINK to the frontend.
//
// Flow analysis (verified 2026-06-26):
//   1. PublishSessionRevisionEnvelope → bus.Publish(EnvelopeTypeRunStatus env)
//   2. EventBusConsumer.handleEnvelope receives the envelope
//   3. c.buffer.Handle(env) is a NO-OP (ProvideEnvelopeBuffer returns nil —
//      Phase 5 Blocker E removed the in-process replay buffer as write-only
//      with no reader)
//   4. envelopeToDomainEvent(env) converts to DomainEvent with
//      Type=DomainEventType("run_status") — an UNDEFINED DomainEventType value
//      (the enum only declares runner_completion/state_delta/error/graph_*/
//      text_delta/tool_call/tool_result; "run_status" is not in the list)
//   5. handleDomainEvent's switch only matches DomainEventRunnerCompletion
//      and DomainEventStateDelta → the run_status event is SILENTLY DROPPED
//
// Consequence: the frontend's `web/src/features/chat/inboundSyncEnvelope.ts`
// `activitySessionRevision` / `isSessionRevisionSyncActivity` helpers (which
// read `ev.activity.meta.session_revision`) are unreachable from the backend
// — no code path ever populates `meta.session_revision` on an ActivityEvent.
// The Bump* half of these helpers remains effective (it updates the DB
// session_revision counter via SessionRevisionBumper), but the Publish* half
// is dead.
//
// Why migration is blocked:
//   - Moving session_revision.go to biz.ActivityEventBus requires importing
//     internal/biz from internal/event — but internal/biz already imports
//     internal/event (event_bus_consumer.go:9), creating a circular import.
//   - ActivityEvent.Kind has no enum for "session_revision" (closest match
//     is ActivityKindSession + Stage="run_status", but the run_status
//     ActivityEvent publisher at internal/service/run_status_publish.go
//     PublishRunStatusFull does not populate `meta.session_revision`).
//
// Architecture recommendation (deferred — requires design session):
//   Option A (preferred): relocate session_revision.go to internal/biz
//     package and rewrite the publish path to ActivityEvent
//     (Kind=ActivityKindSession, Stage="session_revision_sync",
//     Domain=ActivityDomainSystem, Meta={"session_revision":N,"status":...,
//     "turn_id":...,"run_id":...,"skip_hydrate":bool}). Update service and
//     team callers to use biz.BumpAndPublishSessionRevision*.
//   Option B: define a SessionRevisionPublisher interface in
//     internal/event/contract that exposes PublishSessionRevision(rev, ...)
//     without referencing Envelope or ActivityEvent; let biz provide the
//     implementation backed by ActivityEventBus.
//   Option C: extend PublishRunStatusFull in run_status_publish.go to
//     accept a session_revision parameter and have service/team callers
//     invoke it directly (delete session_revision.go entirely).
//
// Until the architecture decision is made, the publish path is retained
// as-is to preserve bump semantics and avoid touching service/team callers.

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
func BumpAndPublishSessionRevision(ctx context.Context, bumper SessionRevisionBumper, bus Bus, sessionID, runID, turnID, source string, lg loggateway.Logger) {
	bumpAndPublishSessionRevision(ctx, bumper, bus, sessionID, runID, turnID, source, SessionRunStatusCompleted, lg)
}

// BumpAndPublishSessionRevisionSync bumps revision after user message persist (status=sync).
// Web clients must hydrate incrementally without treating this as turn completion.
func BumpAndPublishSessionRevisionSync(ctx context.Context, bumper SessionRevisionBumper, bus Bus, sessionID, runID, turnID, source string, lg loggateway.Logger) {
	bumpAndPublishSessionRevision(ctx, bumper, bus, sessionID, runID, turnID, source, SessionRunStatusSync, lg)
}

func bumpAndPublishSessionRevision(ctx context.Context, bumper SessionRevisionBumper, bus Bus, sessionID, runID, turnID, source, status string, lg loggateway.Logger) {
	if bumper == nil || bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	rev, err := bumper.BumpSessionRevision(ctx, sessionID)
	if err != nil {
		lg.Warn("session_revision bump failed",
			loggateway.StepID("session.revision.bump"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err))
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
