package biz

import "time"

// This file provides factory functions for all v2 event types and implements
// the EntityID() method required by the Event interface.
//
// Since event struct fields (taskID, spiritSessionID, occurredAt) are unexported,
// cross-package construction via struct literal is impossible. These factories
// are the only way for external packages (e.g. internal/agent/v2) to create events.
//
// Conventions:
//   - For entity-embedded events: factory takes the entity, derives taskID and
//     spiritSessionID from the entity's fields, defaults occurredAt to time.Now().
//   - For StepStreamingEvent (no embedded entity): factory takes all fields explicitly.
//   - For PlanStep events: PlanStep has no SpiritSessionID field, so the factory
//     accepts it as a separate parameter.

// === Task event factories ===

// NewTaskCreatedEvent constructs a TaskCreatedEvent from a Task.
// taskID defaults to task.ID; spiritSessionID defaults to task.SessionID.
func NewTaskCreatedEvent(task Task) *TaskCreatedEvent {
	return &TaskCreatedEvent{
		taskID:          task.ID,
		spiritSessionID: task.SessionID,
		Task:            task,
		occurredAt:      time.Now(),
	}
}

// NewTaskUpdatedEvent constructs a TaskUpdatedEvent from a Task.
func NewTaskUpdatedEvent(task Task) *TaskUpdatedEvent {
	return &TaskUpdatedEvent{
		taskID:          task.ID,
		spiritSessionID: task.SessionID,
		Task:            task,
		occurredAt:      time.Now(),
	}
}

// NewTaskCompletedEvent constructs a TaskCompletedEvent from a Task.
func NewTaskCompletedEvent(task Task) *TaskCompletedEvent {
	return &TaskCompletedEvent{
		taskID:          task.ID,
		spiritSessionID: task.SessionID,
		Task:            task,
		occurredAt:      time.Now(),
	}
}

// NewTaskFailedEvent constructs a TaskFailedEvent from a Task.
func NewTaskFailedEvent(task Task) *TaskFailedEvent {
	return &TaskFailedEvent{
		taskID:          task.ID,
		spiritSessionID: task.SessionID,
		Task:            task,
		occurredAt:      time.Now(),
	}
}

// === Turn event factories ===

// NewTurnStartedEvent constructs a TurnStartedEvent from a Turn.
func NewTurnStartedEvent(turn Turn) *TurnStartedEvent {
	return &TurnStartedEvent{
		taskID:          turn.TaskID,
		spiritSessionID: turn.SpiritSessionID,
		TurnID:          turn.ID,
		Turn:            turn,
		occurredAt:      time.Now(),
	}
}

// NewTurnCompletedEvent constructs a TurnCompletedEvent from a Turn.
func NewTurnCompletedEvent(turn Turn) *TurnCompletedEvent {
	return &TurnCompletedEvent{
		taskID:          turn.TaskID,
		spiritSessionID: turn.SpiritSessionID,
		TurnID:          turn.ID,
		Turn:            turn,
		occurredAt:      time.Now(),
	}
}

// NewTurnFailedEvent constructs a TurnFailedEvent from a Turn.
func NewTurnFailedEvent(turn Turn) *TurnFailedEvent {
	return &TurnFailedEvent{
		taskID:          turn.TaskID,
		spiritSessionID: turn.SpiritSessionID,
		TurnID:          turn.ID,
		Turn:            turn,
		occurredAt:      time.Now(),
	}
}

// === Step event factories ===

// NewStepCreatedEvent constructs a StepCreatedEvent from a Step.
func NewStepCreatedEvent(step Step) *StepCreatedEvent {
	return &StepCreatedEvent{
		taskID:          step.TaskID,
		spiritSessionID: step.SpiritSessionID,
		Step:            step,
		occurredAt:      time.Now(),
	}
}

// NewStepStreamingEvent constructs a StepStreamingEvent.
// Since this event has no embedded entity, all identifying fields are required.
func NewStepStreamingEvent(spiritSessionID, taskID, stepID, deltaField, deltaChunk string) *StepStreamingEvent {
	return &StepStreamingEvent{
		taskID:          taskID,
		spiritSessionID: spiritSessionID,
		StepID:          stepID,
		DeltaField:      deltaField,
		DeltaChunk:      deltaChunk,
		occurredAt:      time.Now(),
	}
}

// NewStepUpdatedEvent constructs a StepUpdatedEvent from a Step.
func NewStepUpdatedEvent(step Step) *StepUpdatedEvent {
	return &StepUpdatedEvent{
		taskID:          step.TaskID,
		spiritSessionID: step.SpiritSessionID,
		Step:            step,
		occurredAt:      time.Now(),
	}
}

// NewStepCompletedEvent constructs a StepCompletedEvent from a Step.
func NewStepCompletedEvent(step Step) *StepCompletedEvent {
	return &StepCompletedEvent{
		taskID:          step.TaskID,
		spiritSessionID: step.SpiritSessionID,
		Step:            step,
		occurredAt:      time.Now(),
	}
}

// NewStepFailedEvent constructs a StepFailedEvent from a Step.
func NewStepFailedEvent(step Step) *StepFailedEvent {
	return &StepFailedEvent{
		taskID:          step.TaskID,
		spiritSessionID: step.SpiritSessionID,
		Step:            step,
		occurredAt:      time.Now(),
	}
}

// === TeamStage event factories ===

// NewTeamStageCreatedEvent constructs a TeamStageCreatedEvent.
// spiritSessionID is derived from TeamStage.SessionID (= spirit_session_id).
func NewTeamStageCreatedEvent(ts TeamStage) *TeamStageCreatedEvent {
	return &TeamStageCreatedEvent{
		taskID:          ts.TaskID,
		spiritSessionID: ts.SessionID,
		TeamStage:       ts,
		occurredAt:      time.Now(),
	}
}

// NewTeamStageUpdatedEvent constructs a TeamStageUpdatedEvent.
func NewTeamStageUpdatedEvent(ts TeamStage) *TeamStageUpdatedEvent {
	return &TeamStageUpdatedEvent{
		taskID:          ts.TaskID,
		spiritSessionID: ts.SessionID,
		TeamStage:       ts,
		occurredAt:      time.Now(),
	}
}

// NewTeamStageCompletedEvent constructs a TeamStageCompletedEvent.
func NewTeamStageCompletedEvent(ts TeamStage) *TeamStageCompletedEvent {
	return &TeamStageCompletedEvent{
		taskID:          ts.TaskID,
		spiritSessionID: ts.SessionID,
		TeamStage:       ts,
		occurredAt:      time.Now(),
	}
}

// NewTeamStageFailedEvent constructs a TeamStageFailedEvent.
func NewTeamStageFailedEvent(ts TeamStage) *TeamStageFailedEvent {
	return &TeamStageFailedEvent{
		taskID:          ts.TaskID,
		spiritSessionID: ts.SessionID,
		TeamStage:       ts,
		occurredAt:      time.Now(),
	}
}

// === TeamRun event factories ===

// NewTeamRunStartedEvent constructs a TeamRunStartedEvent.
func NewTeamRunStartedEvent(tr TeamRun) *TeamRunStartedEvent {
	return &TeamRunStartedEvent{
		taskID:          tr.TaskID,
		spiritSessionID: tr.SpiritSessionID,
		TeamRun:         tr,
		occurredAt:      time.Now(),
	}
}

// NewTeamRunCompletedEvent constructs a TeamRunCompletedEvent.
func NewTeamRunCompletedEvent(tr TeamRun) *TeamRunCompletedEvent {
	return &TeamRunCompletedEvent{
		taskID:          tr.TaskID,
		spiritSessionID: tr.SpiritSessionID,
		TeamRun:         tr,
		occurredAt:      time.Now(),
	}
}

// NewTeamRunFailedEvent constructs a TeamRunFailedEvent.
func NewTeamRunFailedEvent(tr TeamRun) *TeamRunFailedEvent {
	return &TeamRunFailedEvent{
		taskID:          tr.TaskID,
		spiritSessionID: tr.SpiritSessionID,
		TeamRun:         tr,
		occurredAt:      time.Now(),
	}
}

// === MemberSession event factories ===

// NewMemberSessionCreatedEvent constructs a MemberSessionCreatedEvent.
func NewMemberSessionCreatedEvent(ms MemberSession) *MemberSessionCreatedEvent {
	return &MemberSessionCreatedEvent{
		taskID:          ms.TaskID,
		spiritSessionID: ms.SpiritSessionID,
		MemberSession:   ms,
		occurredAt:      time.Now(),
	}
}

// NewMemberSessionUpdatedEvent constructs a MemberSessionUpdatedEvent.
func NewMemberSessionUpdatedEvent(ms MemberSession) *MemberSessionUpdatedEvent {
	return &MemberSessionUpdatedEvent{
		taskID:          ms.TaskID,
		spiritSessionID: ms.SpiritSessionID,
		MemberSession:   ms,
		occurredAt:      time.Now(),
	}
}

// === PlanBoard event factories ===

// NewPlanBoardCreatedEvent constructs a PlanBoardCreatedEvent.
// spiritSessionID is derived from PlanBoard.SessionID (= spirit_session_id).
func NewPlanBoardCreatedEvent(pb PlanBoard) *PlanBoardCreatedEvent {
	return &PlanBoardCreatedEvent{
		taskID:          pb.TaskID,
		spiritSessionID: pb.SessionID,
		PlanBoard:       pb,
		occurredAt:      time.Now(),
	}
}

// NewPlanBoardUpdatedEvent constructs a PlanBoardUpdatedEvent.
func NewPlanBoardUpdatedEvent(pb PlanBoard) *PlanBoardUpdatedEvent {
	return &PlanBoardUpdatedEvent{
		taskID:          pb.TaskID,
		spiritSessionID: pb.SessionID,
		PlanBoard:       pb,
		occurredAt:      time.Now(),
	}
}

// === PlanStep event factories ===
// PlanStep has no SpiritSessionID field; the factory accepts it as a separate parameter.

// NewPlanStepStartedEvent constructs a PlanStepStartedEvent.
func NewPlanStepStartedEvent(ps PlanStep, spiritSessionID string) *PlanStepStartedEvent {
	return &PlanStepStartedEvent{
		taskID:          ps.TaskID,
		spiritSessionID: spiritSessionID,
		PlanStep:        ps,
		occurredAt:      time.Now(),
	}
}

// NewPlanStepCompletedEvent constructs a PlanStepCompletedEvent.
func NewPlanStepCompletedEvent(ps PlanStep, spiritSessionID string) *PlanStepCompletedEvent {
	return &PlanStepCompletedEvent{
		taskID:          ps.TaskID,
		spiritSessionID: spiritSessionID,
		PlanStep:        ps,
		occurredAt:      time.Now(),
	}
}

// NewPlanStepFailedEvent constructs a PlanStepFailedEvent.
func NewPlanStepFailedEvent(ps PlanStep, spiritSessionID string) *PlanStepFailedEvent {
	return &PlanStepFailedEvent{
		taskID:          ps.TaskID,
		spiritSessionID: spiritSessionID,
		PlanStep:        ps,
		occurredAt:      time.Now(),
	}
}

// NewPlanStepSkippedEvent constructs a PlanStepSkippedEvent.
func NewPlanStepSkippedEvent(ps PlanStep, spiritSessionID, reason string) *PlanStepSkippedEvent {
	return &PlanStepSkippedEvent{
		taskID:          ps.TaskID,
		spiritSessionID: spiritSessionID,
		PlanStep:        ps,
		Reason:          reason,
		occurredAt:      time.Now(),
	}
}

// NewPlanStepUpdatedEvent constructs a PlanStepUpdatedEvent.
func NewPlanStepUpdatedEvent(ps PlanStep, spiritSessionID string) *PlanStepUpdatedEvent {
	return &PlanStepUpdatedEvent{
		taskID:          ps.TaskID,
		spiritSessionID: spiritSessionID,
		PlanStep:        ps,
		occurredAt:      time.Now(),
	}
}

// === EntityID() method implementations ===
// Returns the ID of the primary entity carried by the event.
// Used by the Sequencer's dead-letter ring for entity-ID-based deduplication.

func (e *TaskCreatedEvent) EntityID() string   { return e.Task.ID }
func (e *TaskUpdatedEvent) EntityID() string   { return e.Task.ID }
func (e *TaskCompletedEvent) EntityID() string { return e.Task.ID }
func (e *TaskFailedEvent) EntityID() string    { return e.Task.ID }

func (e *TurnStartedEvent) EntityID() string   { return e.TurnID }
func (e *TurnCompletedEvent) EntityID() string { return e.TurnID }
func (e *TurnFailedEvent) EntityID() string    { return e.TurnID }

func (e *StepCreatedEvent) EntityID() string   { return e.Step.ID }
func (e *StepStreamingEvent) EntityID() string { return e.StepID }
func (e *StepUpdatedEvent) EntityID() string   { return e.Step.ID }
func (e *StepCompletedEvent) EntityID() string { return e.Step.ID }
func (e *StepFailedEvent) EntityID() string    { return e.Step.ID }

func (e *TeamStageCreatedEvent) EntityID() string   { return e.TeamStage.ID }
func (e *TeamStageUpdatedEvent) EntityID() string   { return e.TeamStage.ID }
func (e *TeamStageCompletedEvent) EntityID() string { return e.TeamStage.ID }
func (e *TeamStageFailedEvent) EntityID() string    { return e.TeamStage.ID }

func (e *TeamRunStartedEvent) EntityID() string   { return e.TeamRun.ID }
func (e *TeamRunCompletedEvent) EntityID() string { return e.TeamRun.ID }
func (e *TeamRunFailedEvent) EntityID() string    { return e.TeamRun.ID }

func (e *MemberSessionCreatedEvent) EntityID() string { return e.MemberSession.ID }
func (e *MemberSessionUpdatedEvent) EntityID() string { return e.MemberSession.ID }

func (e *PlanBoardCreatedEvent) EntityID() string { return e.PlanBoard.ID }
func (e *PlanBoardUpdatedEvent) EntityID() string { return e.PlanBoard.ID }

func (e *PlanStepStartedEvent) EntityID() string   { return e.PlanStep.ID }
func (e *PlanStepCompletedEvent) EntityID() string { return e.PlanStep.ID }
func (e *PlanStepFailedEvent) EntityID() string    { return e.PlanStep.ID }
func (e *PlanStepSkippedEvent) EntityID() string   { return e.PlanStep.ID }
func (e *PlanStepUpdatedEvent) EntityID() string   { return e.PlanStep.ID }

func (e *GraphStageCreatedEvent) EntityID() string     { return e.GraphStage.ID }
func (e *GraphStageUpdatedEvent) EntityID() string     { return e.GraphStage.ID }
func (e *GraphStageCompletedEvent) EntityID() string   { return e.GraphStage.ID }
func (e *GraphStageFailedEvent) EntityID() string      { return e.GraphStage.ID }
func (e *GraphStageInterruptedEvent) EntityID() string { return e.GraphStage.ID }
func (e *GraphNodeUpdatedEvent) EntityID() string      { return e.GraphNode.ID }

// === GraphStage event factories ===
// GraphStage has SpiritSessionID via SessionID field (= spirit_session_id).

// NewGraphStageCreatedEvent constructs a GraphStageCreatedEvent.
// spiritSessionID is derived from GraphStage.SessionID (= spirit_session_id).
func NewGraphStageCreatedEvent(gs GraphStage) *GraphStageCreatedEvent {
	return &GraphStageCreatedEvent{
		taskID:          gs.TaskID,
		spiritSessionID: gs.SessionID,
		GraphStage:      gs,
		occurredAt:      time.Now(),
	}
}

// NewGraphStageUpdatedEvent constructs a GraphStageUpdatedEvent.
func NewGraphStageUpdatedEvent(gs GraphStage) *GraphStageUpdatedEvent {
	return &GraphStageUpdatedEvent{
		taskID:          gs.TaskID,
		spiritSessionID: gs.SessionID,
		GraphStage:      gs,
		occurredAt:      time.Now(),
	}
}

// NewGraphStageCompletedEvent constructs a GraphStageCompletedEvent.
func NewGraphStageCompletedEvent(gs GraphStage) *GraphStageCompletedEvent {
	return &GraphStageCompletedEvent{
		taskID:          gs.TaskID,
		spiritSessionID: gs.SessionID,
		GraphStage:      gs,
		occurredAt:      time.Now(),
	}
}

// NewGraphStageFailedEvent constructs a GraphStageFailedEvent.
func NewGraphStageFailedEvent(gs GraphStage) *GraphStageFailedEvent {
	return &GraphStageFailedEvent{
		taskID:          gs.TaskID,
		spiritSessionID: gs.SessionID,
		GraphStage:      gs,
		occurredAt:      time.Now(),
	}
}

// NewGraphStageInterruptedEvent constructs a GraphStageInterruptedEvent.
func NewGraphStageInterruptedEvent(gs GraphStage) *GraphStageInterruptedEvent {
	return &GraphStageInterruptedEvent{
		taskID:          gs.TaskID,
		spiritSessionID: gs.SessionID,
		GraphStage:      gs,
		occurredAt:      time.Now(),
	}
}

// NewGraphNodeUpdatedEvent constructs a GraphNodeUpdatedEvent.
// GraphNode has no TaskID/SpiritSessionID fields; the factory accepts them as parameters.
// graphStageID is used to look up the parent GraphStage for spiritSessionID/taskID derivation.
func NewGraphNodeUpdatedEvent(gn GraphNode, taskID, spiritSessionID string) *GraphNodeUpdatedEvent {
	return &GraphNodeUpdatedEvent{
		taskID:          taskID,
		spiritSessionID: spiritSessionID,
		GraphNode:       gn,
		occurredAt:      time.Now(),
	}
}
