package service

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

func TestisTurnMessageQueued(t *testing.T) {
	if !isTurnMessageQueued(ErrTurnMessageQueued) {
		t.Fatal("expected ErrTurnMessageQueued to match")
	}
	if isTurnMessageQueued(turnBusyError()) {
		t.Fatal("turnBusyError should not match queued sentinel")
	}
}

func TestisTurnBusyError(t *testing.T) {
	if !isTurnBusyError(turnBusyError()) {
		t.Fatal("expected busy error")
	}
	if isTurnBusyError(ErrTurnMessageQueued) {
		t.Fatal("queued should not match busy")
	}
}

func TestTurnResultToNative_completed(t *testing.T) {
	tr := biz.TurnResult{
		Outcome:      biz.TurnOutcomeCompleted,
		UserMsg:      biz.ChatMessage{ID: "u1"},
		AssistantMsg: biz.ChatMessage{ID: "a1", ContentMarkdown: "hi"},
	}
	got, err := turnResultToNative(tr, nil)
	if err != nil || got.Outcome != biz.NativeTurnOutcomeCompleted {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestTurnResultToNative_queued(t *testing.T) {
	tr := biz.TurnResult{Outcome: biz.TurnOutcomeQueued, PendingID: "p1"}
	got, err := turnResultToNative(tr, ErrTurnMessageQueued)
	if !isTurnMessageQueued(err) || got.PendingID != "p1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestTurnResultToNative_error_non_queued(t *testing.T) {
	tr := biz.TurnResult{
		Outcome: biz.TurnOutcomeFailed,
		UserMsg: biz.ChatMessage{ID: "u1"},
	}
	someErr := errors.New("generic failure")
	got, err := turnResultToNative(tr, someErr)
	if err != someErr {
		t.Fatalf("err = %v, want same as input", err)
	}
	if got.Outcome != biz.NativeTurnOutcomeFailed {
		t.Fatalf("outcome = %q, want %q", got.Outcome, biz.NativeTurnOutcomeFailed)
	}
}

func TestTurnResultToNative_default_outcome(t *testing.T) {
	tr := biz.TurnResult{Outcome: "unknown_outcome"}
	got, err := turnResultToNative(tr, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != biz.NativeTurnOutcomeFailed {
		t.Fatalf("outcome = %q, want %q for unknown", got.Outcome, biz.NativeTurnOutcomeFailed)
	}
}

func TestTurnResultToNative_rejected(t *testing.T) {
	tr := biz.TurnResult{Outcome: biz.TurnOutcomeRejected}
	got, err := turnResultToNative(tr, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != biz.NativeTurnOutcomeFailed {
		t.Fatalf("outcome = %q, want %q for rejected default", got.Outcome, biz.NativeTurnOutcomeFailed)
	}
}
