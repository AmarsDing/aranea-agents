package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestRalphLoopConfigFromSettings_Disabled(t *testing.T) {
	cfg, err := RalphLoopConfigFromSettings(&biz.AgentRuntimeSettings{})
	if err != nil || cfg != nil {
		t.Fatalf("expected nil config, got cfg=%v err=%v", cfg, err)
	}
}

func TestRalphLoopConfigFromSettings_PromiseOnly(t *testing.T) {
	cfg, err := RalphLoopConfigFromSettings(&biz.AgentRuntimeSettings{
		RalphLoopMaxIterations:     3,
		RalphLoopCompletionPromise: "DONE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.MaxIterations != 3 || cfg.CompletionPromise != "DONE" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.PromiseTagOpen != defaultRalphPromiseTagOpen {
		t.Fatalf("tag open: %q", cfg.PromiseTagOpen)
	}
}

func TestRalphLoopConfigFromSettings_MissingStopCondition(t *testing.T) {
	_, err := RalphLoopConfigFromSettings(&biz.AgentRuntimeSettings{
		RalphLoopMaxIterations: 2,
	})
	if err == nil {
		t.Fatal("expected error without promise or verify command")
	}
}

func TestResolveRalphLoopTurn_SkipErr(t *testing.T) {
	rl := ResolveRalphLoopTurn(&biz.AgentRuntimeSettings{RalphLoopMaxIterations: 1})
	if rl.Config != nil || rl.SkipErr == nil {
		t.Fatalf("expected skip: cfg=%v err=%v", rl.Config, rl.SkipErr)
	}
}

func TestResolveRalphLoopTurn_OK(t *testing.T) {
	rl := ResolveRalphLoopTurn(&biz.AgentRuntimeSettings{
		RalphLoopMaxIterations:     3,
		RalphLoopCompletionPromise: "DONE",
	})
	if rl.SkipErr != nil || rl.Config == nil {
		t.Fatalf("unexpected: cfg=%v err=%v", rl.Config, rl.SkipErr)
	}
}
