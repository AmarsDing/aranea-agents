package service

import (
	"testing"
)

func TestIsTurnMessageQueued(t *testing.T) {
	if !isTurnMessageQueued(ErrTurnMessageQueued) {
		t.Fatal("expected ErrTurnMessageQueued to match")
	}
	if isTurnMessageQueued(turnBusyError()) {
		t.Fatal("turnBusyError should not match queued sentinel")
	}
}

func TestIsTurnBusyError(t *testing.T) {
	if !isTurnBusyError(turnBusyError()) {
		t.Fatal("expected busy error")
	}
	if isTurnBusyError(ErrTurnMessageQueued) {
		t.Fatal("queued should not match busy")
	}
}
