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
	BumpSessionRevision(ctx context.Context, sessionID string)
}

// chatTurnEventPublisher implements turnEventPublisher.
//
// Part of the TECH-DEBT(BL8) resolution: separating event publishing from
// the orchestrator's core turn logic.
//
// activityBus carries ActivityEvents (Kind=task/error) replacing the legacy
// error envelope. Session revision bumps go directly to the SessionRevisionBumper
// (DB increment) — the legacy Envelope publish path was removed in ADR-03
// Phase 5 Blocker D (SessionBus had no live subscriber).
type chatTurnEventPublisher struct {
	sessions    biz.SessionTurnManager
	activityBus biz.ActivityEventBus
	lg          loggateway.Logger
}

func newChatTurnEventPublisher(sessions biz.SessionTurnManager, activityBus biz.ActivityEventBus, lg loggateway.Logger) *chatTurnEventPublisher {
	return &chatTurnEventPublisher{sessions: sessions, activityBus: activityBus, lg: lg}
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

// BumpSessionRevision bumps the session revision counter after a turn or
// message persist.
func (p *chatTurnEventPublisher) BumpSessionRevision(ctx context.Context, sessionID string) {
	if p == nil || p.sessions == nil {
		return
	}
	event.BumpSessionRevision(ctx, p.sessions, sessionID, p.lg)
}
