package biz

import (
	"context"
	"testing"
)

func TestCheckRouteHonored_FIT_ROUTE_1(t *testing.T) {
	team := RouteDecision{Lane: RouteLanePlanTeam, Mode: "dag"}
	if err := CheckRouteHonored(team, StrategyDirect, 0, ""); err == nil {
		t.Fatal("PlanTeam + direct/0 must fail FIT-ROUTE-1")
	}
	if err := CheckRouteHonored(team, StrategyDirect, 0, "needs_clarification"); err != nil {
		t.Fatalf("clarification is a legal veto: %v", err)
	}
	if err := CheckRouteHonored(team, StrategyDAG, 0, "decompose_failed"); err != nil {
		t.Fatalf("fail-closed non-direct is honored: %v", err)
	}
	if err := CheckRouteHonored(team, StrategyDAG, 0, DecomposeReasonDeferred); err == nil {
		t.Fatal("PlanTeam + deferred/0 must fail FIT-ROUTE-1")
	}
	if err := CheckRouteHonored(RouteDecision{}, StrategyDirect, 0, ""); err != nil {
		t.Fatalf("unspecified lane is not constrained: %v", err)
	}
}

func TestCheckRunHonesty_FIT_OBS_1(t *testing.T) {
	if err := CheckRunHonesty("orphaned", 0, "2026-08-31T00:00:00Z"); err == nil {
		t.Fatal("orphaned with error_count=0 must fail")
	}
	if err := CheckRunHonesty("failed", 1, ""); err == nil {
		t.Fatal("failed without finished_at must fail")
	}
	if err := CheckRunHonesty("orphaned", 1, "2026-08-31T00:00:00Z"); err != nil {
		t.Fatalf("honest orphaned row: %v", err)
	}
	if err := CheckRunHonesty("ok", 0, ""); err != nil {
		t.Fatalf("ok is not constrained: %v", err)
	}
	if err := CheckRunHonesty("cancelled", 0, "x"); err != nil {
		t.Fatalf("cancelled is user-initiated: %v", err)
	}
}

func TestCheckBudgetTrim_FIT_BUDGET_1(t *testing.T) {
	if err := CheckBudgetTrim(116000, 60000, false); err == nil {
		t.Fatal("S05-shaped 116k vs 60k hard with no trim must fail")
	}
	if err := CheckBudgetTrim(116000, 60000, true); err != nil {
		t.Fatalf("trimmed call is honest: %v", err)
	}
	if err := CheckBudgetTrim(14000, 60000, false); err != nil {
		t.Fatalf("under hard is fine: %v", err)
	}
}

func TestCheckCompressionInflection(t *testing.T) {
	// S09-shaped monotone climb through 50% of 128k window.
	series := []int{6874, 20000, 40000, 68835}
	if err := CheckCompressionInflection(series, 128000); err == nil {
		t.Fatal("monotone climb past 50% window must fail")
	}
	withDrop := []int{6874, 40000, 20000, 25000}
	if err := CheckCompressionInflection(withDrop, 128000); err != nil {
		t.Fatalf("drop before 50%% is an inflection: %v", err)
	}
}

func TestRouteDecisionContextRoundTrip(t *testing.T) {
	d := RouteDecision{Lane: RouteLanePlanTeam, Mode: "dag", ForcePlanning: true}
	ctx := ContextWithRouteDecision(context.Background(), d)
	got, ok := RouteDecisionFromContext(ctx)
	if !ok || got.Lane != RouteLanePlanTeam || got.Mode != "dag" {
		t.Fatalf("roundtrip = %+v ok=%v", got, ok)
	}
	if _, ok := RouteDecisionFromContext(context.Background()); ok {
		t.Fatal("absent ctx must not report specified")
	}
	if _, ok := RouteDecisionFromContext(ContextWithRouteDecision(context.Background(), RouteDecision{})); ok {
		t.Fatal("unspecified lane must not report specified")
	}
}
