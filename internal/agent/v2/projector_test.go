package v2

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// capturingSequencer collects published events for test assertions.
type capturingSequencer struct {
	events []biz.Event
}

func (c *capturingSequencer) Publish(_ context.Context, e biz.Event) {
	c.events = append(c.events, e)
}

func testProjector() (*ActivityProjector, *capturingSequencer) {
	capture := &capturingSequencer{}
	p := NewActivityProjector(capture, nil, nil)
	p.OnTurnStart(context.Background(), ProjectMeta{
		TaskID:          "task-1",
		TurnID:          "turn-1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		AgentKey:        "agent-1",
	})
	return p, capture
}

func TestEmitNotice(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil // only care about notice events

	err := p.EmitNotice(context.Background(), "model switched to gpt-4", "model_router")
	if err != nil {
		t.Fatalf("EmitNotice returned error: %v", err)
	}

	// Expect 2 events: StepCreatedEvent + StepCompletedEvent
	if len(capture.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(capture.events))
	}

	created, ok := capture.events[0].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected StepCreatedEvent, got %T", capture.events[0])
	}
	if created.Step.Kind != biz.StepKindNotice {
		t.Errorf("expected kind=notice, got %s", created.Step.Kind)
	}
	if created.Step.Content != "model switched to gpt-4" {
		t.Errorf("expected content set on creation, got %s", created.Step.Content)
	}
	if created.Step.NoticeType != "model_router" {
		t.Errorf("expected noticeType=model_router, got %s", created.Step.NoticeType)
	}

	completed, ok := capture.events[1].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[1])
	}
	if completed.Step.Status != biz.StepStatusCompleted {
		t.Errorf("expected status=completed, got %s", completed.Step.Status)
	}
	if completed.Step.Content != "model switched to gpt-4" {
		t.Errorf("expected content set on completion, got %s", completed.Step.Content)
	}
	if completed.Step.NoticeType != "model_router" {
		t.Errorf("expected noticeType preserved, got %s", completed.Step.NoticeType)
	}
}

func TestEmitConfirmRequest(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	stepID, err := p.EmitConfirmRequest(context.Background(), biz.ActivityConfirmParams{
		ToolName:      "shell",
		ToolArguments: `{"cmd":"rm -rf /"}`,
		Content:       "Allow shell execution?",
	})
	if err != nil {
		t.Fatalf("EmitConfirmRequest returned error: %v", err)
	}
	if stepID == "" {
		t.Fatal("expected non-empty stepID")
	}

	// Expect 2 events: StepCreatedEvent (pending) + StepUpdatedEvent (tool_blocked)
	if len(capture.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(capture.events))
	}

	created, ok := capture.events[0].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected StepCreatedEvent, got %T", capture.events[0])
	}
	if created.Step.Kind != biz.StepKindConfirm {
		t.Errorf("expected kind=confirm, got %s", created.Step.Kind)
	}
	if created.Step.Status != biz.StepStatusPending {
		t.Errorf("expected initial status=pending, got %s", created.Step.Status)
	}

	updated, ok := capture.events[1].(*biz.StepUpdatedEvent)
	if !ok {
		t.Fatalf("expected StepUpdatedEvent, got %T", capture.events[1])
	}
	if updated.Step.Status != biz.StepStatusToolBlocked {
		t.Errorf("expected status=tool_blocked, got %s", updated.Step.Status)
	}
	if updated.Step.ToolName != "shell" {
		t.Errorf("expected toolName=shell, got %s", updated.Step.ToolName)
	}
	if updated.Step.Content != "Allow shell execution?" {
		t.Errorf("expected content set, got %s", updated.Step.Content)
	}
	if string(updated.Step.ToolArgs) != `{"cmd":"rm -rf /"}` {
		t.Errorf("expected toolArgs set, got %s", string(updated.Step.ToolArgs))
	}
}

func TestEmitConfirmResult(t *testing.T) {
	p, _ := testProjector()

	// Test approved
	stepID, _ := p.EmitConfirmRequest(context.Background(), biz.ActivityConfirmParams{
		ToolName: "shell",
		Content:  "Allow?",
	})
	capture := p.seq.(*capturingSequencer)
	capture.events = nil

	err := p.EmitConfirmResult(context.Background(), stepID, true)
	if err != nil {
		t.Fatalf("EmitConfirmResult approved returned error: %v", err)
	}
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event for approved, got %d", len(capture.events))
	}
	completed, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if completed.Step.Status != biz.StepStatusCompleted {
		t.Errorf("expected status=completed, got %s", completed.Step.Status)
	}
	if completed.Step.Kind != biz.StepKindConfirm {
		t.Errorf("expected kind=confirm, got %s", completed.Step.Kind)
	}

	// Test denied
	stepID2, _ := p.EmitConfirmRequest(context.Background(), biz.ActivityConfirmParams{
		ToolName: "shell",
		Content:  "Allow again?",
	})
	capture.events = nil

	err = p.EmitConfirmResult(context.Background(), stepID2, false)
	if err != nil {
		t.Fatalf("EmitConfirmResult denied returned error: %v", err)
	}
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event for denied, got %d", len(capture.events))
	}
	cancelled, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if cancelled.Step.Status != biz.StepStatusCancelled {
		t.Errorf("expected status=cancelled, got %s", cancelled.Step.Status)
	}

	// Test not found
	err = p.EmitConfirmResult(context.Background(), "nonexistent", true)
	if err == nil {
		t.Error("expected error for nonexistent stepID")
	}
}
