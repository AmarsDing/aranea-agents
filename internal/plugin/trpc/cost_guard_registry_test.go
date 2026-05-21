package plugintrpc

import "testing"

func TestCostGuardBudgetRegistry_ScopeIsolation(t *testing.T) {
	t.Parallel()
	reg := NewCostGuardBudgetRegistry()
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
		budgets: NewCostGuardBudgetRegistry(),
		active: []runtimeEntry{
			{key: "cost_guard", scope: "global", costGuard: &CostGuardConfig{}},
		},
	}
	if got := rt.CostGuardScopeForAgent("agent-42"); got != "agent-42" {
		t.Fatalf("global cost_guard should bucket by agent, got %q", got)
	}
}
