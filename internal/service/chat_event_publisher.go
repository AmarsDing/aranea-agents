package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
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
type chatTurnEventPublisher struct {
	sessions biz.SessionTurnManager
	bus      event.Bus
	lg       loggateway.Logger
}

func newChatTurnEventPublisher(sessions biz.SessionTurnManager, bus event.Bus, lg loggateway.Logger) *chatTurnEventPublisher {
	return &chatTurnEventPublisher{sessions: sessions, bus: bus, lg: lg}
}

// Compile-time interface check.
var _ turnEventPublisher = (*chatTurnEventPublisher)(nil)

// PublishTurnFailure emits a WS-visible error envelope for turn failures.
func (p *chatTurnEventPublisher) PublishTurnFailure(sessionID, runID, source string, err error, pendingID string) {
	if p == nil || p.bus == nil || err == nil {
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
	p.bus.Publish(publishCtx, env)
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
