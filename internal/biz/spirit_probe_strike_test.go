package biz

import "testing"

func TestSpiritOrchestration_ProbeStrikeClearOnStop(t *testing.T) {
	o := &SpiritOrchestration{}
	if o.noteProbeFailure("team-1") != 1 {
		t.Fatal("first strike")
	}
	if o.ProbeStrikeCount("team-1") != 1 {
		t.Fatal("count")
	}
	o.Stop()
	if o.ProbeStrikeCount("team-1") != 0 {
		t.Fatalf("expected cleared after Stop, got %d", o.ProbeStrikeCount("team-1"))
	}
}