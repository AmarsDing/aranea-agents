package team

import (
	"os"
	"strings"
)

// TeamGraphRuntimeEnabled reports whether this run should use the GraphAgent runtime path.
// Requires ARANEA_TEAM_GRAPH_RUNTIME=1 and definition runtime_engine=graph (or team_graph_runtime=true).
func TeamGraphRuntimeEnabled(def Definition) bool {
	if !envTeamGraphRuntimeGate() {
		return false
	}
	if def.TeamGraphRuntime {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(def.RuntimeEngine), "graph")
}

func envTeamGraphRuntimeGate() bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_TEAM_GRAPH_RUNTIME"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// SupportsTeamGraphRuntimeMode lists team modes that can execute via compiled GraphAgent (Phase 3).
func SupportsTeamGraphRuntimeMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "sequential", "parallel", "coordinator", "critic_loop", "adaptive", "swarm":
		return true
	default:
		return false
	}
}
