package biz

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EventDeliveryOutboxRow is one durable critical-delivery record (B-06).
// Payload is the marshaled WS v2_event envelope bytes (ready to push on replay).
type EventDeliveryOutboxRow struct {
	ID          string
	SessionID   string
	Seq         int64
	EventID     string
	Kind        string
	EntityID    string
	Payload     []byte
	PublishedAt *time.Time
	CreatedAt   time.Time
}

// EventDeliveryOutboxRepo persists critical v2 events for reconnect replay.
// Stability: Evolving
type EventDeliveryOutboxRepo interface {
	Insert(ctx context.Context, row EventDeliveryOutboxRow) error
	MarkPublished(ctx context.Context, id string, publishedAt time.Time) error
	// ListAfter returns rows for sessionID with seq strictly greater than the
	// cursor. When afterEventID is non-empty, the cursor seq is resolved from
	// that event_id (unknown id → empty result). When afterEventID is empty and
	// afterSeq > 0, afterSeq is used directly. limit <= 0 defaults to 100.
	ListAfter(ctx context.Context, sessionID, afterEventID string, afterSeq int64, limit int) ([]EventDeliveryOutboxRow, error)
}

// EventSeq extracts the session-scoped sequence from a typed v2 event payload.
// Returns 0 when the event carries no sequence (caller may assign one).
func EventSeq(e Event) int64 {
	if e == nil {
		return 0
	}
	switch ev := e.(type) {
	case *TaskCreatedEvent:
		return ev.Task.Seq
	case *TaskUpdatedEvent:
		return ev.Task.Seq
	case *TaskCompletedEvent:
		return ev.Task.Seq
	case *TaskFailedEvent:
		return ev.Task.Seq
	case *TurnStartedEvent:
		return ev.Turn.Seq
	case *TurnCompletedEvent:
		return ev.Turn.Seq
	case *TurnFailedEvent:
		return ev.Turn.Seq
	case *StepCreatedEvent:
		return ev.Step.Seq
	case *StepUpdatedEvent:
		return ev.Step.Seq
	case *StepCompletedEvent:
		return ev.Step.Seq
	case *StepFailedEvent:
		return ev.Step.Seq
	case *StepStreamingEvent:
		return 0
	case *TeamStageCreatedEvent:
		return ev.TeamStage.Seq
	case *TeamStageUpdatedEvent:
		return ev.TeamStage.Seq
	case *TeamStageCompletedEvent:
		return ev.TeamStage.Seq
	case *TeamStageFailedEvent:
		return ev.TeamStage.Seq
	case *TeamRunStartedEvent:
		return ev.TeamRun.Seq
	case *TeamRunCompletedEvent:
		return ev.TeamRun.Seq
	case *TeamRunFailedEvent:
		return ev.TeamRun.Seq
	case *MemberSessionCreatedEvent:
		return ev.MemberSession.Seq
	case *MemberSessionUpdatedEvent:
		return ev.MemberSession.Seq
	case *PlanBoardCreatedEvent:
		return ev.PlanBoard.Seq
	case *PlanBoardUpdatedEvent:
		return ev.PlanBoard.Seq
	case *PlanStepStartedEvent:
		return ev.PlanStep.Seq
	case *PlanStepCompletedEvent:
		return ev.PlanStep.Seq
	case *PlanStepFailedEvent:
		return ev.PlanStep.Seq
	case *PlanStepSkippedEvent:
		return ev.PlanStep.Seq
	case *PlanStepUpdatedEvent:
		return ev.PlanStep.Seq
	case *GraphStageCreatedEvent:
		return ev.GraphStage.Seq
	case *GraphStageUpdatedEvent:
		return ev.GraphStage.Seq
	case *GraphStageCompletedEvent:
		return ev.GraphStage.Seq
	case *GraphStageFailedEvent:
		return ev.GraphStage.Seq
	case *GraphStageInterruptedEvent:
		return ev.GraphStage.Seq
	case *SystemNoticeEvent:
		return ev.Seq
	case *ActivityBridgeEvent:
		return ev.Event.Activity.Seq
	default:
		return 0
	}
}

// SetEventSeq assigns seq onto events that support an explicit Seq field
// (currently SystemNoticeEvent). Entity-backed events keep their embedded Seq.
func SetEventSeq(e Event, seq int64) {
	if e == nil || seq <= 0 {
		return
	}
	if ev, ok := e.(*SystemNoticeEvent); ok {
		ev.Seq = seq
	}
}

// DeliveryEventID builds a stable cursor id for outbox + WS last_event_id.
// Format: v2:{session}:{seq}:{kind}:{entity}
func DeliveryEventID(e Event, seq int64) string {
	if e == nil {
		return ""
	}
	sessionID := strings.TrimSpace(e.SpiritSessionID())
	kind := string(e.EventKind())
	entityID := strings.TrimSpace(e.EntityID())
	return fmt.Sprintf("v2:%s:%d:%s:%s", sessionID, seq, kind, entityID)
}
