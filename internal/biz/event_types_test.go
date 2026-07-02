package biz

import (
	"testing"
	"time"
)

func TestEvent_InterfaceCompliance(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		&TaskCreatedEvent{taskID: "t1", spiritSessionID: "s1", Task: Task{ID: "t1", SessionID: "s1", Seq: 1, CreatedAt: now}},
		&TurnStartedEvent{taskID: "t1", spiritSessionID: "s1", TurnID: "turn1", Turn: Turn{ID: "turn1", TaskID: "t1", Seq: 2, StartedAt: now}},
		&StepCreatedEvent{taskID: "t1", spiritSessionID: "s1", Step: Step{ID: "st1", TurnID: "turn1", Kind: StepKindThinking, Seq: 1, Status: StepStatusRunning}},
		&StepStreamingEvent{taskID: "t1", spiritSessionID: "s1", StepID: "st1", DeltaField: "content", DeltaChunk: "hello"},
		&StepCompletedEvent{taskID: "t1", spiritSessionID: "s1", Step: Step{ID: "st1", Status: StepStatusCompleted}},
		&TeamStageCreatedEvent{taskID: "t1", spiritSessionID: "s1", TeamStage: TeamStage{ID: "ts1", TaskID: "t1", Seq: 5, Status: TeamStageStatusPending}},
		&PlanStepStartedEvent{taskID: "t1", spiritSessionID: "s1", PlanStep: PlanStep{ID: "ps1", Status: PlanStepStatusRunning}},
	}
	for i, e := range events {
		if e.EventKind() == "" {
			t.Fatalf("event[%d]: empty EventKind", i)
		}
		if e.SpiritSessionID() == "" {
			t.Fatalf("event[%d]: empty SpiritSessionID", i)
		}
		if e.TaskID() == "" {
			t.Fatalf("event[%d]: empty TaskID", i)
		}
	}
}

func TestEventKind_Constants(t *testing.T) {
	if EventKindTaskCreated != "task.created" {
		t.Fatalf("expected task.created, got %s", EventKindTaskCreated)
	}
	if EventKindStepStreaming != "step.streaming" {
		t.Fatalf("expected step.streaming, got %s", EventKindStepStreaming)
	}
	if EventKindPlanStepCompleted != "plan_step.completed" {
		t.Fatalf("expected plan_step.completed, got %s", EventKindPlanStepCompleted)
	}
}
