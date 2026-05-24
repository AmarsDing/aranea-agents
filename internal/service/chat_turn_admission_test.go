package service

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime/turn"
)

func TestNativeResultFromAdmissionVerdict_queued(t *testing.T) {
	result, err := nativeResultFromAdmissionVerdict(turn.AdmissionVerdict{
		Action:    turn.AdmissionQueued,
		PendingID: "p-1",
	})
	if !IsTurnMessageQueued(err) {
		t.Fatalf("err=%v", err)
	}
	if result.Outcome != biz.NativeTurnOutcomeQueued || result.PendingID != "p-1" {
		t.Fatalf("got=%+v", result)
	}
}

func TestNativeResultFromAdmissionVerdict_busy(t *testing.T) {
	_, err := nativeResultFromAdmissionVerdict(turn.AdmissionVerdict{Action: turn.AdmissionRejectBusy})
	if !IsTurnBusyError(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestNativeResultFromAdmissionVerdict_enqueueError(t *testing.T) {
	want := errors.New("db")
	_, err := nativeResultFromAdmissionVerdict(turn.AdmissionVerdict{
		Action: turn.AdmissionRejectEnqueue,
		Err:    want,
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}
