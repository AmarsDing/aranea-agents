package tools

import "testing"

func TestProductionAllowAdHocHTTP(t *testing.T) {
	if ProductionAllowAdHocHTTP(true, false) {
		t.Fatal("expected false when platform disabled")
	}
	if !ProductionAllowAdHocHTTP(true, true) {
		t.Fatal("expected true when both enabled")
	}
	if ProductionAllowAdHocHTTP(false, true) {
		t.Fatal("expected false when server not configured")
	}
}
