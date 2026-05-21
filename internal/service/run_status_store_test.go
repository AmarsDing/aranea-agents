package service

import "testing"

func TestTerminalRunStatus(t *testing.T) {
	if !terminalRunStatus("completed") {
		t.Fatal("completed should be terminal")
	}
	if terminalRunStatus("awaiting_user") {
		t.Fatal("awaiting_user should persist")
	}
}
