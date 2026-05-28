package team

import (
	"os"
	"strings"
)

func TeamGraphRuntimeEnabled(def Definition) bool {
	if !envTeamGraphRuntimeGate() {
		return false
	}
	if envTeamNativeForced() {
		return false
	}
	return true
}

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

func envTeamNativeForced() bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_TEAM_NATIVE"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func SupportsTeamGraphRuntimeMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "sequential", "parallel", "coordinator", "critic_loop", "adaptive", "swarm":
		return true
	default:
		return false
	}
}
