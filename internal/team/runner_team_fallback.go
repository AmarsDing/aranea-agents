package team

import (
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
)

// TeamGraphRuntimeEnabledForTeam determines whether graph runtime should be
// attempted for a given team based on its definition and canary configuration.
func TeamGraphRuntimeEnabledForTeam(def Definition, teamID string) bool {
	engine := strings.TrimSpace(def.RuntimeEngine)
	if strings.EqualFold(engine, "native") {
		return envTeamNativeForced()
	}
	if strings.EqualFold(engine, "graph") {
		return true
	}
	// Default: use graph canary bucket
	return teamInGraphCanaryBucket(teamID, teamGraphCanaryPercent())
}

// envTeamNativeForced returns true if ARANEA_TEAM_NATIVE=1 is set.
func envTeamNativeForced() bool {
	return os.Getenv("ARANEA_TEAM_NATIVE") == "1"
}

// teamGraphCanaryPercent returns the percentage of teams that should use
// graph runtime (0-100). Defaults to 100 (all teams use graph).
func teamGraphCanaryPercent() int {
	v := os.Getenv("ARANEA_TEAM_GRAPH_CANARY_PERCENT")
	if v == "" {
		return 100
	}
	p, err := strconv.Atoi(v)
	if err != nil || p < 0 || p > 100 {
		return 100
	}
	return p
}

// teamInGraphCanaryBucket determines if a team falls within the canary
// percentage bucket for graph runtime.
func teamInGraphCanaryBucket(teamID string, percent int) bool {
	if percent >= 100 {
		return true
	}
	if percent <= 0 {
		return false
	}
	// Simple hash-based bucketing
	h := uint32(0)
	for _, c := range teamID {
		h = h*31 + uint32(c)
	}
	return int(h%100) < percent
}

// teamNativeAllowedForCanaryHoldout returns true if this team is in the
// canary holdout group that should still use native runtime.
func teamNativeAllowedForCanaryHoldout(def Definition, teamID string) bool {
	engine := strings.TrimSpace(def.RuntimeEngine)
	if strings.EqualFold(engine, "native") {
		return true
	}
	// Teams outside the graph canary bucket can use native as fallback
	return !teamInGraphCanaryBucket(teamID, teamGraphCanaryPercent())
}

// nativeRuntimeMetricReason returns the metric label for native runtime usage.
func nativeRuntimeMetricReason(graphAttempted, canaryHoldout bool) string {
	switch {
	case canaryHoldout:
		return "canary_holdout"
	case graphAttempted:
		return "graph_fallback"
	default:
		return "no_graph_configured"
	}
}

// TurnDeadlineDuration returns the turn deadline from the team definition.
func TurnDeadlineDuration(def Definition) time.Duration {
	if def.TimeoutSeconds > 0 {
		return time.Duration(def.TimeoutSeconds) * time.Second
	}
	return 0
}
