package team

import (
	"hash/fnv"
	"os"
	"strconv"
	"strings"
)

const teamGraphCanaryPercentEnv = "ARANEA_TEAM_GRAPH_CANARY_PERCENT"

// teamGraphCanaryPercent returns 0–100; empty means 100 (full Graph rollout).
func teamGraphCanaryPercent() int {
	v := strings.TrimSpace(os.Getenv(teamGraphCanaryPercentEnv))
	if v == "" {
		return 100
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 100
	}
	if n > 100 {
		return 100
	}
	return n
}

// teamInGraphCanaryBucket assigns teams to Graph canary by stable hash(teamID) % 100 < percent.
func teamInGraphCanaryBucket(teamID string, percent int) bool {
	if percent >= 100 {
		return true
	}
	if percent <= 0 {
		return false
	}
	id := strings.TrimSpace(teamID)
	if id == "" {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return int(h.Sum32()%100) < percent
}

// TeamGraphRuntimeEnabledForTeam applies platform gate, definition opt-out, and canary bucketing.
// Explicit runtime_engine=graph always enables Graph (manual canary enrollment per Runbook).
func TeamGraphRuntimeEnabledForTeam(def Definition, teamID string) bool {
	if !TeamGraphRuntimeEnabled(def) {
		return false
	}
	engine := strings.ToLower(strings.TrimSpace(def.RuntimeEngine))
	if engine == "graph" || def.TeamGraphRuntime {
		return true
	}
	pct := teamGraphCanaryPercent()
	if pct >= 100 {
		return true
	}
	return teamInGraphCanaryBucket(teamID, pct)
}

// teamNativeAllowedForCanaryHoldout permits Native execution for teams outside the Graph canary bucket.
func teamNativeAllowedForCanaryHoldout(def Definition, teamID string) bool {
	if envTeamNativeForced() {
		return true
	}
	pct := teamGraphCanaryPercent()
	if pct >= 100 || pct <= 0 {
		return false
	}
	engine := strings.ToLower(strings.TrimSpace(def.RuntimeEngine))
	if engine == "graph" || def.TeamGraphRuntime {
		return false
	}
	if engine == "native" {
		return true
	}
	return !teamInGraphCanaryBucket(teamID, pct)
}

// nativeRuntimeMetricReason labels Prometheus native path selection.
func nativeRuntimeMetricReason(graphAttempted, canaryHoldout bool) string {
	if graphAttempted {
		return "native_fallback"
	}
	if canaryHoldout {
		return "native_canary_holdout"
	}
	return "native_emergency"
}
