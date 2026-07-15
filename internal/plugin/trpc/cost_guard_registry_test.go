package plugintrpc

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestCostGuardBudgetRegistry_ScopeIsolation(t *testing.T) {
	t.Parallel()
	reg := NewCostGuardBudgetRegistry(loggateway.NewNoop())
	a := reg.TrackerForScope("agent-a")
	b := reg.TrackerForScope("agent-b")
	if a == b {
		t.Fatal("expected distinct trackers per scope")
	}
	cfg := CostGuardConfig{DailyTokenBudget: 1000, FallbackModel: "cheap"}
	a.TryConsume(cfg.DailyTokenBudget, 900)
	targetA := ResolveCostGuardTarget("base", cfg, 200, a)
	targetB := ResolveCostGuardTarget("base", cfg, 200, b)
	if targetA != "cheap" {
		t.Fatalf("agent-a should hit budget, got %q", targetA)
	}
	if targetB != "" {
		t.Fatalf("agent-b should remain on base (no fallback), got %q", targetB)
	}
}

func TestCostGuardScopeForAgent_GlobalPluginPerAgent(t *testing.T) {
	t.Parallel()
	rt := &Runtime{
		budgets: NewCostGuardBudgetRegistry(loggateway.NewNoop()),
		activeByWS: map[string][]runtimeEntry{
			"": {{key: "cost_guard", scope: "global", costGuard: &CostGuardConfig{}}},
		},
	}
	if got := rt.CostGuardScopeForAgent("agent-42"); got != "default:agent-42" {
		t.Fatalf("global cost_guard should bucket by workspace:agent, got %q", got)
	}
}

func TestCostGuardScopeForAgentInWorkspace_Isolation(t *testing.T) {
	t.Parallel()
	rt := &Runtime{
		budgets: NewCostGuardBudgetRegistry(loggateway.NewNoop()),
		activeByWS: map[string][]runtimeEntry{
			"": {{key: "cost_guard", scope: "global", costGuard: &CostGuardConfig{}}},
		},
	}
	a := rt.CostGuardScopeForAgentInWorkspace("ws-a", "agent-1")
	b := rt.CostGuardScopeForAgentInWorkspace("ws-b", "agent-1")
	if a == b {
		t.Fatalf("expected workspace-isolated scopes, got %q and %q", a, b)
	}
	if a != "ws-a:agent-1" || b != "ws-b:agent-1" {
		t.Fatalf("unexpected scopes a=%q b=%q", a, b)
	}
}
