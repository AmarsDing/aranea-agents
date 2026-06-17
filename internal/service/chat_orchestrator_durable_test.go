package service

import (
	"context"
	"testing"

	"aranea-agents/internal/event"
)

func TestDurableResumeTurnCtxFrom_appliesSpec(t *testing.T) {
	ctx := event.WithDurableResume(context.Background(), event.DurableResumeSpec{
		SessionRunID:     "sr-1",
		TurnID:           "turn-1",
		RuntimeRunID:     "rt-1",
		TrpcInvocationID: "inv-1",
		DialogMode:       "react",
		Provider:         "openai",
		Model:            "gpt-4",
	})
	got := durableResumeTurnCtxFrom(ctx, "new-run", "default", "prov", "mod")
	if !got.active {
		t.Fatal("expected active durable ctx")
	}
	if got.runID != "rt-1" {
		t.Fatalf("runID=%q", got.runID)
	}
	if got.dialogMode != "react" || got.provider != "openai" || got.model != "gpt-4" {
		t.Fatalf("mode/provider/model=%q/%q/%q", got.dialogMode, got.provider, got.model)
	}
	if got.spec.SessionRunID != "sr-1" {
		t.Fatalf("sessionRunID=%q", got.spec.SessionRunID)
	}
}

func TestDurableResumeTurnCtxFrom_plainTurn(t *testing.T) {
	got := durableResumeTurnCtxFrom(context.Background(), "run-a", "dm", "p", "m")
	if got.active {
		t.Fatal("expected inactive")
	}
	if got.runID != "run-a" {
		t.Fatalf("runID=%q", got.runID)
	}
}

func TestDurableResumeRunOpts(t *testing.T) {
	base := durableResumeRunOpts(false, nil)
	if len(base) != 0 {
		t.Fatalf("base=%v", base)
	}
	with := durableResumeRunOpts(true, nil)
	if len(with) != 2 {
		t.Fatalf("with=%v", with)
	}
}
