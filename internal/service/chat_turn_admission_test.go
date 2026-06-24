package service

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime/turn"
)

func TestTurnResultFromAdmissionVerdict_queued(t *testing.T) {
	result, err := turnResultFromAdmissionVerdict(turn.AdmissionVerdict{
		Action:    turn.AdmissionQueued,
		PendingID: "p-1",
	})
	if !isTurnMessageQueued(err) {
		t.Fatalf("err=%v", err)
	}
	if result.Outcome != biz.TurnOutcomeQueued || result.PendingID != "p-1" {
		t.Fatalf("got=%+v", result)
	}
}

func TestTurnResultFromAdmissionVerdict_busy(t *testing.T) {
	_, err := turnResultFromAdmissionVerdict(turn.AdmissionVerdict{Action: turn.AdmissionRejectBusy})
	if !isTurnBusyError(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestTurnResultFromAdmissionVerdict_enqueueError(t *testing.T) {
	want := errors.New("db")
	_, err := turnResultFromAdmissionVerdict(turn.AdmissionVerdict{
		Action: turn.AdmissionRejectEnqueue,
		Err:    want,
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}
