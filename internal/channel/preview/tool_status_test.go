package preview

import "testing"

func TestToolStatusInFlight(t *testing.T) {
	if !ToolStatusInFlight(ToolStatusCalling) || ToolStatusInFlight(ToolStatusOK) {
		t.Fatal("in-flight mismatch")
	}
}

func TestIsTerminalToolStatus(t *testing.T) {
	if !IsTerminalToolStatus(ToolStatusOK) || IsTerminalToolStatus(ToolStatusCalling) {
		t.Fatal("terminal mismatch")
	}
}
