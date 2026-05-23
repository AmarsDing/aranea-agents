package team

import (
	"fmt"
	"testing"
)

func TestEnvTeamGraphCanaryPercent(t *testing.T) {
	t.Setenv(teamGraphCanaryPercentEnv, "")
	if got := teamGraphCanaryPercent(); got != 100 {
		t.Fatalf("empty=%d want 100", got)
	}
	t.Setenv(teamGraphCanaryPercentEnv, "5")
	if got := teamGraphCanaryPercent(); got != 5 {
		t.Fatalf("5=%d", got)
	}
	t.Setenv(teamGraphCanaryPercentEnv, "150")
	if got := teamGraphCanaryPercent(); got != 100 {
		t.Fatalf("clamp=%d", got)
	}
}

func TestTeamInGraphCanaryBucket_stable(t *testing.T) {
	a := teamInGraphCanaryBucket("team-alpha", 50)
	b := teamInGraphCanaryBucket("team-alpha", 50)
	if a != b {
		t.Fatal("bucket assignment must be stable")
	}
	if !teamInGraphCanaryBucket("any", 100) {
		t.Fatal("100% should include all")
	}
	if teamInGraphCanaryBucket("any", 0) {
		t.Fatal("0% should exclude all")
	}
}

func TestTeamGraphRuntimeEnabledForTeam_canaryExplicitGraph(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "")
	t.Setenv("ARANEA_TEAM_NATIVE", "")
	t.Setenv(teamGraphCanaryPercentEnv, "5")

	def := Definition{RuntimeEngine: "graph"}
	if !TeamGraphRuntimeEnabledForTeam(def, "team-outside-bucket") {
		t.Fatal("explicit graph should always enable")
	}
}

func TestTeamGraphRuntimeEnabledForTeam_canaryHoldout(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "")
	t.Setenv("ARANEA_TEAM_NATIVE", "")
	t.Setenv(teamGraphCanaryPercentEnv, "1")

	// Find a team id outside 1% bucket.
	var holdoutID string
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("team-canary-probe-%d", i)
		if !teamInGraphCanaryBucket(id, 1) {
			holdoutID = id
			break
		}
	}
	if holdoutID == "" {
		t.Skip("could not find holdout team id in probe range")
	}

	def := Definition{Mode: "sequential"}
	if TeamGraphRuntimeEnabledForTeam(def, holdoutID) {
		t.Fatalf("holdout %q should not get graph at 1%% canary", holdoutID)
	}
	if !teamNativeAllowedForCanaryHoldout(def, holdoutID) {
		t.Fatalf("holdout %q should allow native", holdoutID)
	}
}

func TestNativeRuntimeMetricReason(t *testing.T) {
	if nativeRuntimeMetricReason(true, false) != "native_fallback" {
		t.Fatal("fallback")
	}
	if nativeRuntimeMetricReason(false, true) != "native_canary_holdout" {
		t.Fatal("holdout")
	}
	if nativeRuntimeMetricReason(false, false) != "native_emergency" {
		t.Fatal("emergency")
	}
}
