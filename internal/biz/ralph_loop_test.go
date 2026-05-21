package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestRalphLoopConfigured(t *testing.T) {
	if biz.RalphLoopConfigured(nil) {
		t.Fatal("nil settings should not be configured")
	}
	if biz.RalphLoopConfigured(&biz.AgentRuntimeSettings{}) {
		t.Fatal("empty settings should not be configured")
	}
	if !biz.RalphLoopConfigured(&biz.AgentRuntimeSettings{RalphLoopCompletionPromise: "DONE"}) {
		t.Fatal("promise should configure ralph loop")
	}
}

func TestValidateRalphLoopSettings(t *testing.T) {
	if err := biz.ValidateRalphLoopSettings(nil); err != nil {
		t.Fatalf("nil: %v", err)
	}
	if err := biz.ValidateRalphLoopSettings(&biz.AgentRuntimeSettings{}); err != nil {
		t.Fatalf("empty: %v", err)
	}
	if err := biz.ValidateRalphLoopSettings(&biz.AgentRuntimeSettings{
		RalphLoopMaxIterations: 3,
	}); err == nil {
		t.Fatal("expected error without promise or verify command")
	}
	if err := biz.ValidateRalphLoopSettings(&biz.AgentRuntimeSettings{
		RalphLoopVerifyCommand: "go test ./...",
	}); err != nil {
		t.Fatalf("verify only: %v", err)
	}
	if err := biz.ValidateRalphLoopSettings(&biz.AgentRuntimeSettings{
		RalphLoopMaxIterations:        -1,
		RalphLoopCompletionPromise:    "x",
	}); err == nil {
		t.Fatal("expected negative max_iterations error")
	}
}
