package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
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
	svc := &ChatService{orch: &ChatOrchestrator{}, lg: loggateway.NewNoop()}
	got, err := svc.ExecuteTurn(context.Background(), biz.TurnInput{SessionID: "s1"})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}
