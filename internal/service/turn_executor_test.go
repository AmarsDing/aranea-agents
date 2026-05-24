package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime/turn"
)

func TestTurnExecutor_Execute_rejectsEmptyInput(t *testing.T) {
	o := &ChatOrchestrator{}
	got, err := o.Execute(context.Background(), biz.TurnInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q want failed", got.Outcome)
	}
}

func TestTurnExecutor_ExecuteTurnGateway(t *testing.T) {
	svc := &ChatService{orch: &ChatOrchestrator{}}
	got, err := svc.ExecuteTurn(context.Background(), biz.TurnInput{SessionID: "s1"})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}

func TestTurnExecutor_ClassifyQueuedViaTurnPackage(t *testing.T) {
	native := biz.NativeTurnResult{
		Outcome:   biz.NativeTurnOutcomeQueued,
		PendingID: "pending-1",
	}
	got, err := turn.ClassifyNativeOutcome(native, turn.QueuedSentinel)
	if !errors.Is(err, turn.QueuedSentinel) {
		t.Fatalf("err=%v", err)
	}
	if got.Outcome != biz.TurnOutcomeQueued || got.PendingID != "pending-1" {
		t.Fatalf("got=%+v", got)
	}
}
