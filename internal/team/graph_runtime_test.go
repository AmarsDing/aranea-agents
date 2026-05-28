package team

import (
	"os"
	"testing"
)

func TestTeamGraphRuntimeEnabled_defaultGraph(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "")
	t.Setenv("ARANEA_TEAM_NATIVE", "")
	def := Definition{RuntimeEngine: "graph"}
	if !TeamGraphRuntimeEnabled(def) {
		t.Fatal("graph engine should enable graph runtime by default")
	}
	defEmpty := Definition{}
	if !TeamGraphRuntimeEnabled(defEmpty) {
		t.Fatal("empty runtime_engine should default to graph path")
	}
}

func TestTeamGraphRuntimeEnabled_nativeOptOut(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "")
	t.Setenv("ARANEA_TEAM_NATIVE", "1")
	def := Definition{RuntimeEngine: "graph"}
	if TeamGraphRuntimeEnabled(def) {
		t.Fatal("ARANEA_TEAM_NATIVE=1 should disable graph path regardless of RuntimeEngine")
	}
}

func TestTeamGraphRuntimeEnabled_nativeEnvForcesNativePath(t *testing.T) {
	t.Setenv("ARANEA_TEAM_NATIVE", "1")
	def := Definition{RuntimeEngine: "graph"}
	if TeamGraphRuntimeEnabled(def) {
		t.Fatal("ARANEA_TEAM_NATIVE=1 should skip graph path")
	}
}

func TestEnvTeamGraphRuntimeGate(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "")
	if !envTeamGraphRuntimeGate() {
		t.Fatal("expected graph gate on by default")
	}
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "0")
	if envTeamGraphRuntimeGate() {
		t.Fatal("expected off when explicitly 0")
	}
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "true")
	if !envTeamGraphRuntimeGate() {
		t.Fatal("expected on for true")
	}
	_ = os.Getenv("ARANEA_TEAM_GRAPH_RUNTIME")
}

func TestEnvTeamNativeForced(t *testing.T) {
	t.Setenv("ARANEA_TEAM_NATIVE", "")
	if envTeamNativeForced() {
		t.Fatal("expected off by default")
	}
	t.Setenv("ARANEA_TEAM_NATIVE", "1")
	if !envTeamNativeForced() {
		t.Fatal("expected on for 1")
	}
}

func TestSupportsTeamGraphRuntimeMode(t *testing.T) {
	if !SupportsTeamGraphRuntimeMode("sequential") {
		t.Fatal("sequential should be supported")
	}
	if SupportsTeamGraphRuntimeMode("unknown") {
		t.Fatal("unknown mode should not be supported")
	}
}
