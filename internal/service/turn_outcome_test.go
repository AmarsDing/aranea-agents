package service

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestIsTurnMessageQueued(t *testing.T) {
	if !IsTurnMessageQueued(ErrTurnMessageQueued) {
		t.Fatal("expected ErrTurnMessageQueued to match")
	}
	if IsTurnMessageQueued(turnBusyError()) {
		t.Fatal("turnBusyError should not match queued sentinel")
	}
}

func TestIsTurnBusyError(t *testing.T) {
	if !IsTurnBusyError(turnBusyError()) {
		t.Fatal("expected busy error")
	}
	if IsTurnBusyError(ErrTurnMessageQueued) {
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
	if !IsTurnMessageQueued(err) || got.PendingID != "p1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
