package biz

import (
	"context"
	"fmt"
	"strings"
)

// RouteLane is the single committed routing outcome of a turn.
// Proposers (QuickAssess, PrePlanningGate, intent) may suggest; only the
// orchestrator commits. Plan() / spawn / Spirit consume and must not silently
// rewrite a committed lane (FIT-ROUTE-1).
type RouteLane string

const (
	// RouteLaneUnspecified means the gate did not commit: the LLM may still
	// self-route. This is not "must answer directly".
	RouteLaneUnspecified RouteLane = ""
	RouteLaneDirect      RouteLane = "direct"
	RouteLaneClarify     RouteLane = "clarify"
	RouteLaneHITL        RouteLane = "hitl"
	RouteLaneRefuse      RouteLane = "refuse"
	RouteLanePlanSolo    RouteLane = "plan_solo"
	RouteLanePlanTeam    RouteLane = "plan_team"
)

// RouteDecision is the immutable commit record for one turn.
type RouteDecision struct {
	Lane          RouteLane
	Mode          string // plan_and_execute mode when Lane is PlanTeam (dag/parallel)
	Reason        string
	ForcePlanning bool
	Level         ComplexityLevel
	Score         float64
	TeamEvidence  bool
}

// Specified reports whether a lane was committed (not left to the LLM).
func (d RouteDecision) Specified() bool {
	return d.Lane != RouteLaneUnspecified
}

type routeDecisionCtxKey struct{}

// ContextWithRouteDecision stores the orchestrator-committed route on ctx
// so plan_and_execute / Plan() consume the same decision the gate wrote.
func ContextWithRouteDecision(ctx context.Context, d RouteDecision) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, routeDecisionCtxKey{}, d)
}

// RouteDecisionFromContext reads the committed route. ok is false when absent.
func RouteDecisionFromContext(ctx context.Context) (RouteDecision, bool) {
	if ctx == nil {
		return RouteDecision{}, false
	}
	d, ok := ctx.Value(routeDecisionCtxKey{}).(RouteDecision)
	if !ok || !d.Specified() {
		return RouteDecision{}, false
	}
	return d, true
}

// MergeCommitted copies ctx commit onto input when input has none.
func MergeCommitted(ctx context.Context, input PlanInput) PlanInput {
	if input.Committed.Specified() {
		return input
	}
	if d, ok := RouteDecisionFromContext(ctx); ok {
		input.Committed = d
	}
	return input
}

// CheckRouteHonored is FIT-ROUTE-1: a committed PlanTeam lane must not
// collapse to StrategyDirect with zero subtasks unless a legal veto was
// recorded (clarification). decompose_failed fail-closed must not be Direct.
func CheckRouteHonored(d RouteDecision, strategy OrchestrationStrategy, subtaskCount int, decomposeReason string) error {
	if d.Lane != RouteLanePlanTeam {
		return nil
	}
	reason := strings.ToLower(strings.TrimSpace(decomposeReason))
	if strings.Contains(reason, "needs_clarification") {
		return nil
	}
	if strategy == StrategyDirect && subtaskCount == 0 {
		return fmt.Errorf("FIT-ROUTE-1: committed plan_team yielded direct/0 subtasks (decompose_reason=%q)", decomposeReason)
	}
	return nil
}

// CheckRunHonesty is FIT-OBS-1: failed and orphaned runs must increment
// error_count and carry a finished_at timestamp.
func CheckRunHonesty(status string, errorCountDelta int, finishedAt string) error {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "orphaned":
		if errorCountDelta < 1 {
			return fmt.Errorf("FIT-OBS-1: status %q must increment error_count", status)
		}
		if strings.TrimSpace(finishedAt) == "" {
			return fmt.Errorf("FIT-OBS-1: status %q missing finished_at", status)
		}
	}
	return nil
}

// CheckBudgetTrim is FIT-BUDGET-1 (absolute assembly hard): a call whose
// estimated input exceeds hard must have produced a trim event.
func CheckBudgetTrim(estTokens, hardTokens int, trimmed bool) error {
	if hardTokens <= 0 {
		return nil
	}
	if estTokens > hardTokens && !trimmed {
		return fmt.Errorf("FIT-BUDGET-1: est %d > hard %d with no trim", estTokens, hardTokens)
	}
	return nil
}

// CheckCompressionInflection is FIT-BUDGET-1 (long session): the token
// series must not be strictly non-decreasing through 50% of the window.
// windowTokens<=0 skips the check.
func CheckCompressionInflection(series []int, windowTokens int) error {
	if windowTokens <= 0 || len(series) < 4 {
		return nil
	}
	half := windowTokens / 2
	peak := 0
	hadDrop := false
	for _, n := range series {
		if n > peak {
			peak = n
		}
		if peak > 0 && n < peak {
			hadDrop = true
		}
		if n >= half && !hadDrop {
			return fmt.Errorf("FIT-BUDGET-1: tokens reached %d (≥50%% of window %d) with no compression inflection", n, windowTokens)
		}
	}
	return nil
}
