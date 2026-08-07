// Package planner exposes a planner selection helper that bridges
// biz.AgentRuntimeSettings (plannerKind) to trpc-agent-go planner
// implementations.
package planner

import (
	"strings"

	"aranea-agents/internal/agent/a2ui"

	trpcplanner "trpc.group/trpc-go/trpc-agent-go/planner"
)

// DialogModeSelects reports whether the dialog mode can influence planner
// selection for the given planner kind. When kind explicitly selects a
// planner (react/a2ui/builtin), the dialog mode has no build-time effect;
// otherwise the "plan" dialog mode activates the builtin planner (see Select).
// Used by the agent build cache key to reduce dialog mode to its build effect.
func DialogModeSelects(plannerKind string) bool {
	switch strings.ToLower(strings.TrimSpace(plannerKind)) {
	case "react", "a2ui", "builtin":
		return false
	}
	return true
}

// Select returns the appropriate Planner for the given dialog mode, planner kind,
// and optional planner_config_json (see docs/需求/39 planner.design.md).
//
// plannerKind values: "" | "builtin" | "react" | "a2ui"
//
// Legacy behaviour: when plannerKind is empty and dialogMode is "plan",
// the builtin planner is used (backward compatible with S1 wiring).
func Select(dialogMode, plannerKind, plannerConfigJSON string, pipeline *a2ui.Pipeline) trpcplanner.Planner {
	kind := strings.ToLower(strings.TrimSpace(plannerKind))
	if p := buildPlanner(kind, plannerConfigJSON, pipeline); p != nil {
		return p
	}
	// Legacy: "plan" dialog mode implicitly activates the builtin planner.
	if strings.EqualFold(strings.TrimSpace(dialogMode), "plan") {
		return buildBuiltin(plannerConfigJSON)
	}
	return nil
}
