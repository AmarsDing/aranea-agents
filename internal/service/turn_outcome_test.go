package service

import (
	"testing"
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
