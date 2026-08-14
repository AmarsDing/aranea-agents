package service

import (
	"testing"
)

func TestTerminalRunStatus_AwaitingUserPersists(t *testing.T) {
	if terminalRunStatus("awaiting_user") {
		t.Fatal("awaiting_user should persist in session state")
	}
}
