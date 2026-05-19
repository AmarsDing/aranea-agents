package biz

import "testing"

func TestToolRequiresConfirmation(t *testing.T) {
	catalog := Tool{RequiresConfirmation: true}
	if !ToolRequiresConfirmation(catalog, ToolAgentOverride{}, false) {
		t.Fatal("catalog flag")
	}
	catalog = Tool{}
	ov := ToolAgentOverride{RequiresConfirmation: true}
	if !ToolRequiresConfirmation(catalog, ov, true) {
		t.Fatal("override flag")
	}
	if ToolRequiresConfirmation(Tool{}, ToolAgentOverride{}, true) {
		t.Fatal("expected false")
	}
}
