package v2

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// CompatAdapter translates v2 Events to v1 ActivityEvents so the legacy
// frontend continues to work during Phase 2 (frontend rewrite).
//
// Lifecycle:
//   - Phase 1: adapter is active; v1 frontend receives legacy + new events
//     (legacy frontend ignores the new events it doesn't recognize).
//   - Phase 2: v1 frontend is rewritten; adapter is removed.
//
// Stability: evolving — TODO(Phase 2): remove this entire file.
type CompatAdapter struct {
	v1Bus biz.ActivityEventBus // legacy bus (reaches v1 WS subscribers)
	lg    loggateway.Logger
}

// NewCompatAdapter constructs an adapter that publishes v1 ActivityEvents
// derived from v2 Events.
func NewCompatAdapter(v1Bus biz.ActivityEventBus) *CompatAdapter {
	return &CompatAdapter{
		v1Bus: v1Bus,
		lg:    loggateway.NewNoop(),
	}
}

// WithLogger sets a non-noop logger (used in production).
func (a *CompatAdapter) WithLogger(lg loggateway.Logger) *CompatAdapter {
	a.lg = lg.With(loggateway.Domain("compat_adapter"))
	return a
}

// PublishV1 converts a v2 Event to a v1 ActivityEvent and publishes it.
// Unknown event types are silently dropped (logged at Debug).
func (a *CompatAdapter) PublishV1(ctx context.Context, e biz.Event) {
	v1, ok := a.convert(e)
	if !ok {
		a.lg.Debug("compat adapter: no v1 mapping for event",
			loggateway.Str("kind", string(e.EventKind())))
		return
	}
	a.v1Bus.Publish(ctx, v1)
}

// convert maps a v2 Event to a v1 ActivityEvent.
// Returns (zero, false) if no mapping exists for the event type.
//
// Deviation 6: ActivityEvent uses the Event field (not Type).
// Deviation 2: StepStreamingEvent uses DeltaChunk (not Delta).
// Deviation 4: PlanStep uses PlanID (not PlanBoardID).
func (a *CompatAdapter) convert(e biz.Event) (biz.ActivityEvent, bool) {
	spiritID := e.SpiritSessionID()
	taskID := e.TaskID()
	now := e.OccurredAt()

	switch ev := e.(type) {
	// === Task events → v1 ActivityKind=task ===
	case *biz.TaskCreatedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventCreated,
			Activity: biz.Activity{
				ID:        ev.Task.ID,
				Kind:      biz.ActivityKindTask,
				Status:    biz.ActivityStatusRunning,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
			},
		}, true
	case *biz.TaskCompletedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:        ev.Task.ID,
				Kind:      biz.ActivityKindTask,
				Status:    biz.ActivityStatusCompleted,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
			},
		}, true
	case *biz.TaskFailedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventFailed,
			Activity: biz.Activity{
				ID:        ev.Task.ID,
				Kind:      biz.ActivityKindTask,
				Status:    biz.ActivityStatusFailed,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
			},
		}, true

	// === Step events → v1 ActivityKind (thinking/action/reply/notice/confirm) ===
	case *biz.StepCreatedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventCreated,
			Activity: biz.Activity{
				ID:        ev.Step.ID,
				Kind:      stepKindToV1(ev.Step.Kind),
				Status:    stepStatusToV1(ev.Step.Status),
				SessionID: spiritID,
				TurnID:    ev.Step.TurnID,
				AgentKey:  ev.Step.AuthorAgentKey,
				Timestamp: now,
			},
		}, true
	case *biz.StepStreamingEvent:
		// Deviation 2: DeltaChunk carried in the dedicated field (not Meta).
		// Deviation 6: Event field (not Type).
		return biz.ActivityEvent{
			Event:      biz.ActivityEventStreaming,
			DeltaField: ev.DeltaField,
			DeltaChunk: ev.DeltaChunk,
			Activity: biz.Activity{
				ID:        ev.StepID,
				Kind:      deltaFieldToKind(ev.DeltaField),
				Status:    biz.ActivityStatusRunning,
				SessionID: spiritID,
				Timestamp: now,
			},
		}, true
	case *biz.StepCompletedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:        ev.Step.ID,
				Kind:      stepKindToV1(ev.Step.Kind),
				Status:    biz.ActivityStatusCompleted,
				SessionID: spiritID,
				TurnID:    ev.Step.TurnID,
				AgentKey:  ev.Step.AuthorAgentKey,
				Content:   ev.Step.Content,
				Timestamp: now,
			},
		}, true

	// === TeamStage events → v1 ActivityKind=team_stage ===
	case *biz.TeamStageCreatedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventCreated,
			Activity: biz.Activity{
				ID:        ev.TeamStage.ID,
				Kind:      biz.ActivityKindTeamStage,
				Status:    biz.ActivityStatusRunning,
				SessionID: spiritID,
				TurnID:    ev.TeamStage.TaskID,
				Timestamp: now,
			},
		}, true
	case *biz.TeamStageCompletedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:        ev.TeamStage.ID,
				Kind:      biz.ActivityKindTeamStage,
				Status:    biz.ActivityStatusCompleted,
				SessionID: spiritID,
				TurnID:    ev.TeamStage.TaskID,
				Timestamp: now,
			},
		}, true

	// === PlanStep events → v1 ActivityKind=plan ===
	case *biz.PlanStepStartedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventUpdated,
			Activity: biz.Activity{
				ID:        ev.PlanStep.ID,
				Kind:      biz.ActivityKindPlan,
				Status:    biz.ActivityStatusRunning,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
			},
		}, true
	case *biz.PlanStepCompletedEvent:
		return biz.ActivityEvent{
			Event: biz.ActivityEventUpdated,
			Activity: biz.Activity{
				ID:        ev.PlanStep.ID,
				Kind:      biz.ActivityKindPlan,
				Status:    biz.ActivityStatusCompleted,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
			},
		}, true
	}

	// Other event types (Turn*, TeamRun*, MemberSession*, PlanBoard*,
	// TaskUpdated, StepUpdated/Failed, TeamStageUpdated/Failed, PlanStepFailed/Skipped/Updated)
	// have no direct v1 counterpart and are intentionally NOT translated —
	// the legacy frontend did not visualize them as first-class entities.
	return biz.ActivityEvent{}, false
}

// stepKindToV1 maps biz.StepKind to v1 biz.ActivityKind.
func stepKindToV1(k biz.StepKind) biz.ActivityKind {
	switch k {
	case biz.StepKindThinking:
		return biz.ActivityKindThinking
	case biz.StepKindAction:
		return biz.ActivityKindAction
	case biz.StepKindReply:
		return biz.ActivityKindReply
	case biz.StepKindNotice:
		return biz.ActivityKindNotice
	case biz.StepKindConfirm:
		return biz.ActivityKindConfirm
	}
	return biz.ActivityKindReply // default
}

// stepStatusToV1 maps biz.StepStatus to v1 biz.ActivityStatus.
// Deviation 8: StepStatus has no Thinking/Streaming constants; only the
// actual constants (pending/running/tool_running/tool_blocked/completed/
// failed/cancelled) are mapped.
func stepStatusToV1(s biz.StepStatus) biz.ActivityStatus {
	switch s {
	case biz.StepStatusPending:
		return biz.ActivityStatusPending
	case biz.StepStatusRunning:
		return biz.ActivityStatusRunning
	case biz.StepStatusToolRunning:
		return biz.ActivityStatusToolRunning
	case biz.StepStatusToolBlocked:
		return biz.ActivityStatusToolBlocked
	case biz.StepStatusCompleted:
		return biz.ActivityStatusCompleted
	case biz.StepStatusFailed:
		return biz.ActivityStatusFailed
	case biz.StepStatusCancelled:
		return biz.ActivityStatusCancelled
	}
	return biz.ActivityStatusRunning
}

// deltaFieldToKind maps a streaming delta_field to a v1 ActivityKind.
func deltaFieldToKind(field string) biz.ActivityKind {
	switch field {
	case "reasoning":
		return biz.ActivityKindThinking
	case "content":
		return biz.ActivityKindReply
	case "tool_args", "tool_result":
		return biz.ActivityKindAction
	}
	return biz.ActivityKindReply
}
