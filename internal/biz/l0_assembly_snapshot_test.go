package biz

import (
	"testing"

	"aranea-agents/internal/llmcontext"
)

func TestShouldWriteL0AssemblySnapshot(t *testing.T) {
	settings := &AgentRuntimeSettings{
		L0SnapshotEnabled: true,
		L0SnapshotMode:    "on_warning",
	}
	if ShouldWriteL0AssemblySnapshot(settings, 0.5, false) {
		t.Fatal("expected skip below warning threshold")
	}
	if !ShouldWriteL0AssemblySnapshot(settings, llmcontext.ContextStatusWarningThreshold, false) {
		t.Fatal("expected write at warning threshold")
	}
	settings.L0SnapshotMode = "always"
	if !ShouldWriteL0AssemblySnapshot(settings, 0.1, false) {
		t.Fatal("expected always mode write")
	}
	settings.L0SnapshotMode = "off"
	if ShouldWriteL0AssemblySnapshot(settings, 0.99, false) {
		t.Fatal("expected off mode skip")
	}
	if !ShouldWriteL0AssemblySnapshot(nil, 0.99, true) {
		t.Fatal("expected force debug write")
	}
	if ShouldWriteL0AssemblySnapshot(&AgentRuntimeSettings{L0SnapshotEnabled: false}, 0.99, false) {
		t.Fatal("expected snapshot disabled skip")
	}
}

func TestL0WarningCodesFromRatio(t *testing.T) {
	codes := L0WarningCodesFromRatio(0.95)
	if len(codes) != 1 || codes[0] != "exceeded" {
		t.Fatalf("exceeded: %#v", codes)
	}
	codes = L0WarningCodesFromRatio(0.85)
	if len(codes) != 1 || codes[0] != "critical" {
		t.Fatalf("critical: %#v", codes)
	}
	codes = L0WarningCodesFromRatio(0.65)
	if len(codes) != 1 || codes[0] != "near_limit" {
		t.Fatalf("near_limit: %#v", codes)
	}
	if len(L0WarningCodesFromRatio(0.3)) != 0 {
		t.Fatal("expected no warnings for low ratio")
	}
}
