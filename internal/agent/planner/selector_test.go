package planner_test

import (
	"testing"

	agentplanner "aranea-agents/internal/agent/planner"
	trpcbuiltin "trpc.group/trpc-go/trpc-agent-go/planner/builtin"
)

func TestSelect_react(t *testing.T) {
	if p := agentplanner.Select("", "react", "", nil); p == nil {
		t.Fatal("expected react planner")
	}
}

func TestSelect_legacyPlanDialog(t *testing.T) {
	p := agentplanner.Select("plan", "", "", nil)
	if p == nil {
		t.Fatal("expected builtin planner for legacy plan dialog mode")
	}
	if _, ok := p.(*trpcbuiltin.Planner); !ok {
		t.Fatalf("got %T", p)
	}
}

func TestSelect_builtinConfig(t *testing.T) {
	high := "high"
	cfg := `{"reasoning_effort":"high"}`
	p := agentplanner.Select("", "builtin", cfg, nil)
	if p == nil {
		t.Fatal("expected builtin planner")
	}
	bp, ok := p.(*trpcbuiltin.Planner)
	if !ok {
		t.Fatalf("got %T", p)
	}
	_ = bp
	_ = high
}
