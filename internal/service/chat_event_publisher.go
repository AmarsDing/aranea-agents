package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// turnEventPublisher is the interface for turn-related event publishing.
type turnEventPublisher interface {
	PublishTurnFailure(sessionID, runID, source string, err error, pendingID string)
	BumpSessionRevisionAndPublish(ctx context.Context, sessionID, runID, turnID string)
	BumpSessionRevisionSyncAndPublish(ctx context.Context, sessionID, runID, turnID string)
	NotifySessionRevisionSync(ctx context.Context, sessionID, runID, turnID string)
}

// chatTurnEventPublisher implements turnEventPublisher.
//
// Part of the TECH-DEBT(BL8) resolution: separating event publishing from
// the orchestrator's core turn logic.
//
// activityBus carries ActivityEvents (Kind=task/error) replacing the legacy
// error envelope. bus is retained for session-revision envelopes which are
// blocked from migration by the event→biz circular dependency
// (see session_revision.go).
//
// TECH-DEBT(ADR-03 Phase 5 Blocker D): bus field is NOT vestigial — it is
// actively passed to event.BumpAndPublishSessionRevision* helpers which
// publish session-revision Envelopes. Cannot delete until session-revision
// migration to ActivityEventBus/MonitorEventBus is complete (blocked by
// event→biz circular dependency).
type chatTurnEventPublisher struct {
	sessions    biz.SessionTurnManager
	bus         event.Bus
	activityBus biz.ActivityEventBus
	lg          loggateway.Logger
}

func newChatTurnEventPublisher(sessions biz.SessionTurnManager, bus event.Bus, activityBus biz.ActivityEventBus, lg loggateway.Logger) *chatTurnEventPublisher {
	return &chatTurnEventPublisher{sessions: sessions, bus: bus, activityBus: activityBus, lg: lg}
}

// Compile-time interface check.
var _ turnEventPublisher = (*chatTurnEventPublisher)(nil)

// PublishTurnFailure emits a WS-visible ActivityEvent (Kind=task, Status=failed)
// for turn failures. Replaces the legacy EnvelopeTypeError publish.
func (p *chatTurnEventPublisher) PublishTurnFailure(sessionID, runID, source string, err error, pendingID string) {
	if p == nil || p.activityBus == nil || err == nil {
		return
	}
	code := TurnErrorCodeFromErr(err)
	detail := ""
	if code == "" {
		detail = err.Error()
	}
	envErr := envelopeErrorFromTurn(code, detail)
	publishCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	meta := map[string]any{
		"error_type": "turn_failure",
		"source":     source,
	}
	if code != "" {
		meta["error_code"] = string(code)
	}
	if envErr != nil {
		if envErr.Message != "" {
			meta["error_message"] = envErr.Message
		}
		if envErr.Hint != "" {
			meta["error_hint"] = envErr.Hint
		}
		if envErr.PendingID != "" {
			meta["pending_id"] = envErr.PendingID
		}
	}
	if pendingID != "" {
		meta["pending_id"] = pendingID
	}
	if runID != "" {
		meta["run_id"] = runID
	}

	p.activityBus.Publish(publishCtx, biz.ActivityEvent{
		Event: biz.ActivityEventFailed,
		Activity: biz.Activity{
			ID:        uuid.NewString(),
			Kind:      biz.ActivityKindTask,
			Status:    biz.ActivityStatusFailed,
			SessionID: sessionID,
			Timestamp: time.Now().UTC(),
			Content:   detail,
			Meta:      meta,
		},
		Domain: biz.ActivityDomainChat,
	})
}

// BumpSessionRevisionAndPublish bumps revision after turn completion (status=completed).
func (p *chatTurnEventPublisher) BumpSessionRevisionAndPublish(ctx context.Context, sessionID, runID, turnID string) {
	if p == nil || p.sessions == nil || p.bus == nil {
		return
	}
	event.BumpAndPublishSessionRevision(
		ctx,
		p.sessions,
		p.bus,
		sessionID,
		runID,
		turnID,
		event.EnvelopeSourceFromContext(ctx),
		p.lg,
	)
}

// BumpSessionRevisionSyncAndPublish bumps revision after user message persist (status=sync).
func (p *chatTurnEventPublisher) BumpSessionRevisionSyncAndPublish(ctx context.Context, sessionID, runID, turnID string) {
	if p == nil || p.sessions == nil || p.bus == nil {
		return
	}
	event.BumpAndPublishSessionRevisionSync(
		ctx,
		p.sessions,
		p.bus,
		sessionID,
		runID,
		turnID,
		event.EnvelopeSourceFromContext(ctx),
		p.lg,
	)
}

// NotifySessionRevisionSync notifies Web of the current revision without incrementing (durable resume).
func (p *chatTurnEventPublisher) NotifySessionRevisionSync(ctx context.Context, sessionID, runID, turnID string) {
	if p == nil || p.sessions == nil || p.bus == nil {
		return
	}
	event.NotifySessionRevisionSync(
		ctx,
		p.sessions,
		p.bus,
		sessionID,
		runID,
		turnID,
		event.EnvelopeSourceFromContext(ctx),
	)
}
