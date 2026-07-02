package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type testTurnService struct {
	turn biz.CanonicalTurn
}

func (s *testTurnService) AdmitTurn(_ context.Context, intent biz.TurnIntent) (biz.CanonicalTurn, error) {
	s.turn = biz.CanonicalTurn{
		ID:         "turn-1",
		SessionID:  intent.SessionID,
		Source:     intent.Source,
		TargetType: intent.TargetType,
		Status:     biz.CanonicalTurnStatusRunning,
	}
	return s.turn, nil
}

func (s *testTurnService) CompleteTurn(_ context.Context, turn biz.CanonicalTurn, result biz.TurnResult) (biz.CanonicalTurn, error) {
	turn.Status = biz.CanonicalTurnStatusFromOutcome(result.Outcome)
	s.turn = turn
	return turn, nil
}

func (s *testTurnService) FailTurn(_ context.Context, turn biz.CanonicalTurn, _ error) (biz.CanonicalTurn, error) {
	turn.Status = biz.CanonicalTurnStatusFailed
	s.turn = turn
	return turn, nil
}

type testTurnExecutor struct {
	err error
}

func (e testTurnExecutor) ExecuteTurn(_ context.Context, _ biz.CanonicalTurn, _ biz.TurnInput) (biz.TurnResult, error) {
	if e.err != nil {
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, e.err
	}
	return biz.TurnResult{Outcome: biz.TurnOutcomeCompleted}, nil
}

type testTurnProjector struct {
	events []biz.TurnEvent
}

func (p *testTurnProjector) ProjectTurnEvent(_ context.Context, event biz.TurnEvent) error {
	p.events = append(p.events, event)
	return nil
}

func TestTurnPipelineRunProjectsCompletion(t *testing.T) {
	projector := &testTurnProjector{}
	pipeline := TurnPipeline{
		Service:   &testTurnService{},
		Executor:  testTurnExecutor{},
		Projector: projector,
		Now:       func() time.Time { return time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC) },
		Lg:        loggateway.NewNoop(),
	}

	turn, result, err := pipeline.Run(context.Background(), biz.TurnIntent{
		SessionID: "sess-1",
		Content:   "hello",
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint: biz.EntryPointChannel,
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if turn.Status != biz.CanonicalTurnStatusCompleted {
		t.Fatalf("turn.Status = %q, want completed", turn.Status)
	}
	if result.Outcome != biz.TurnOutcomeCompleted {
		t.Fatalf("result.Outcome = %q, want completed", result.Outcome)
	}
	if len(projector.events) != 2 {
		t.Fatalf("projected %d events, want 2", len(projector.events))
	}
	if projector.events[0].Type != biz.TurnEventQueued || projector.events[1].Type != biz.TurnEventCompleted {
		t.Fatalf("unexpected projected events: %+v", projector.events)
	}
}

func TestTurnPipelineRunProjectsFailure(t *testing.T) {
	projector := &testTurnProjector{}
	pipeline := TurnPipeline{
		Service:   &testTurnService{},
		Executor:  testTurnExecutor{err: errors.New("boom")},
		Projector: projector,
		Lg:        loggateway.NewNoop(),
	}

	turn, _, err := pipeline.Run(context.Background(), biz.TurnIntent{
		SessionID: "sess-1",
		Content:   "hello",
	})
	if err == nil {
		t.Fatal("Run error = nil, want executor error")
	}
	if turn.Status != biz.CanonicalTurnStatusFailed {
		t.Fatalf("turn.Status = %q, want failed", turn.Status)
	}
	if got := projector.events[len(projector.events)-1]; got.Type != biz.TurnEventFailed {
		t.Fatalf("last event = %q, want failed", got.Type)
	}
}
