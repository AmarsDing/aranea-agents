package team

import (
	"os"
	"strings"
)

// TeamGraphRuntimeEnabled reports whether this Team run should compile via GraphAgent (M53 Phase 7 default).
// Graph is on unless runtime_engine=native or ARANEA_TEAM_GRAPH_RUNTIME=0 disables the platform gate.
func TeamGraphRuntimeEnabled(def Definition) bool {
	if !envTeamGraphRuntimeGate() {
		return false
	}
	if envTeamNativeForced() {
		return false
	}
	if def.TeamGraphRuntime {
		return true
	}
	engine := strings.ToLower(strings.TrimSpace(def.RuntimeEngine))
	if engine == "native" {
		return false
	}
	return engine == "graph" || engine == ""
}

// envTeamGraphRuntimeGate is true by default (Phase 6/7). Set ARANEA_TEAM_GRAPH_RUNTIME=0 to disable Graph platform-wide.
func envTeamGraphRuntimeGate() bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_TEAM_GRAPH_RUNTIME"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
}

// envTeamNativeForced enables emergency Native Team execution (BuildTRPCTeam fallback only).
func envTeamNativeForced() bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_TEAM_NATIVE"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// SupportsTeamGraphRuntimeMode lists team modes that can execute via compiled GraphAgent.
func SupportsTeamGraphRuntimeMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "sequential", "parallel", "coordinator", "critic_loop", "adaptive", "swarm":
		return true
	default:
		return false
	}
}
