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
