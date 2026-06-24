package turn

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

type stubRunner struct {
	result biz.TurnResult
	err    error
}

func (s stubRunner) RunWithOutcome(context.Context, biz.TurnInput) (biz.TurnResult, error) {
	return s.result, s.err
}

func TestExecutor_Execute_delegates(t *testing.T) {
	ex := NewExecutor(stubRunner{
		result: biz.TurnResult{
			Outcome:      biz.TurnOutcomeCompleted,
			AssistantMsg: biz.ChatMessage{ContentMarkdown: "ok"},
		},
	})
	got, err := ex.Execute(context.Background(), biz.TurnInput{SessionID: "s1", Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != biz.TurnOutcomeCompleted {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}

func TestExecutor_Execute_nilRunner(t *testing.T) {
	var ex *Executor
	_, err := ex.Execute(context.Background(), biz.TurnInput{SessionID: "s1", Content: "hi"})
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
}

func TestExecutor_Execute_queued(t *testing.T) {
	ex := NewExecutor(stubRunner{
		result: biz.TurnResult{
			Outcome:   biz.TurnOutcomeQueued,
			PendingID: "pq1",
		},
	})
	got, err := ex.Execute(context.Background(), biz.TurnInput{SessionID: "s1", Content: "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != biz.TurnOutcomeQueued || got.PendingID != "pq1" {
		t.Fatalf("got=%+v", got)
	}
}

func TestExecutor_Execute_failed(t *testing.T) {
	ex := NewExecutor(stubRunner{
		result: biz.TurnResult{Outcome: biz.TurnOutcomeFailed},
		err:    errors.New("agent error"),
	})
	got, err := ex.Execute(context.Background(), biz.TurnInput{SessionID: "s1", Content: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}
