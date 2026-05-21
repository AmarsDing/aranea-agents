// Package planner exposes a planner selection helper that bridges
// biz.AgentRuntimeSettings (plannerKind) to trpc-agent-go planner
// implementations.
package planner

import (
	"strings"

	trpcplanner "trpc.group/trpc-go/trpc-agent-go/planner"
)

// Select returns the appropriate Planner for the given dialog mode, planner kind,
// and optional planner_config_json (see docs/需求/39 planner.design.md).
//
// plannerKind values: "" | "builtin" | "react" | "a2ui"
//
// Legacy behaviour: when plannerKind is empty and dialogMode is "plan",
// the builtin planner is used (backward compatible with S1 wiring).
func Select(dialogMode, plannerKind, plannerConfigJSON string) trpcplanner.Planner {
	kind := strings.ToLower(strings.TrimSpace(plannerKind))
	if p := buildPlanner(kind, plannerConfigJSON); p != nil {
		return p
	}
	// Legacy: "plan" dialog mode implicitly activates the builtin planner.
	if strings.EqualFold(strings.TrimSpace(dialogMode), "plan") {
		return buildBuiltin(plannerConfigJSON)
	}
	return nil
}
