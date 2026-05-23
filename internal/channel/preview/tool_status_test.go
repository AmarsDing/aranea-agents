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

func TestNormalizeToolStatus_failed(t *testing.T) {
	if got := NormalizeToolStatus("failed"); got != ToolStatusError {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeToolStatus("success"); got != ToolStatusOK {
		t.Fatalf("got %q", got)
	}
}
