// Package planner exposes a planner selection helper that bridges
// biz.AgentRuntimeSettings (plannerKind) to trpc-agent-go planner
// implementations.
package planner

import (
	"strings"

	trpcplanner "trpc.group/trpc-go/trpc-agent-go/planner"
	trpca2ui "trpc.group/trpc-go/trpc-agent-go/planner/a2ui"
	trpcbuiltin "trpc.group/trpc-go/trpc-agent-go/planner/builtin"
	trpcreact "trpc.group/trpc-go/trpc-agent-go/planner/react"
)

// Select returns the appropriate Planner for the given dialog mode and
// plannerKind.
//
// plannerKind values: "" | "builtin" | "react" | "a2ui"
//
// Legacy behaviour: when plannerKind is empty and dialogMode is "plan",
// the builtin planner is used (backward compatible with S1 wiring).
func Select(dialogMode, plannerKind string) trpcplanner.Planner {
	kind := strings.ToLower(strings.TrimSpace(plannerKind))
	switch kind {
	case "react":
		return trpcreact.New()
	case "a2ui":
		return trpca2ui.New()
	case "builtin":
		return trpcbuiltin.New(trpcbuiltin.Options{})
	default:
		// Legacy: "plan" dialog mode implicitly activates the builtin planner.
		if strings.EqualFold(strings.TrimSpace(dialogMode), "plan") {
			return trpcbuiltin.New(trpcbuiltin.Options{})
		}
		return nil
	}
}
