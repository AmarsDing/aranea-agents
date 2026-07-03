package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// NOTE(Phase3b-D Task 10): chatTurnEventPublisher migrated from v1 ActivityEventBus
// to v2 EventBus. PublishTurnFailure now emits biz.NewTaskFailedEvent.
// DATA LOSS: v2 Task has no Content/Meta fields, so the original error details
// (error_type/source/error_code/error_message/error_hint/pending_id/run_id)
// carried in v1 Activity.Meta are no longer attached to the event.
// Callers needing error context should log it separately or extend the v2
// Task struct in a follow-up task.

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
// eventBus carries v2 Events (Phase 3b-D Task 10): PublishTurnFailure emits
// biz.NewTaskFailedEvent. Session revision bumps go directly to the
// SessionRevisionBumper (DB increment) — the legacy Envelope publish path was
// removed in ADR-03 Phase 5 Blocker D (SessionBus had no live subscriber).
type chatTurnEventPublisher struct {
	sessions biz.SessionTurnManager
	eventBus biz.EventBus
	lg       loggateway.Logger
}

func newChatTurnEventPublisher(sessions biz.SessionTurnManager, eventBus biz.EventBus, lg loggateway.Logger) *chatTurnEventPublisher {
	return &chatTurnEventPublisher{sessions: sessions, eventBus: eventBus, lg: lg}
}

// Compile-time interface check.
var _ turnEventPublisher = (*chatTurnEventPublisher)(nil)

// PublishTurnFailure emits a WS-visible v2 TaskFailedEvent for turn failures.
// Replaces the legacy EnvelopeTypeError publish and the v1 ActivityEvent
// (Kind=task, Status=failed) publish.
//
// Phase 3b-D Task 10: the original v1 event carried rich error metadata in
// Activity.Meta (error_type/source/error_code/error_message/error_hint/
// pending_id/run_id). The v2 Task entity has no Meta field, so these details
// are dropped here. The error message is still available via the caller's
// logging path (turnPipeline.handleStreamError / publishTurnFailure callers
// log the error before calling this method).
func (p *chatTurnEventPublisher) PublishTurnFailure(sessionID, runID, source string, err error, pendingID string) {
	if p == nil || p.eventBus == nil || err == nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	task := biz.Task{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Status:    biz.TaskStatusFailed,
	}
	p.eventBus.Publish(publishCtx, biz.NewTaskFailedEvent(task))
}

// BumpSessionRevision bumps the session revision counter after a turn or
// message persist.
func (p *chatTurnEventPublisher) BumpSessionRevision(ctx context.Context, sessionID string) {
	if p == nil || p.sessions == nil {
		return
	}
	event.BumpSessionRevision(ctx, p.sessions, sessionID, p.lg)
}
