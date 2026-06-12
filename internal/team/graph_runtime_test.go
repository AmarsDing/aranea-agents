package team

import (
	"os"
	"testing"
)

func TestTeamGraphRuntimeEnabled_defaultGraph(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "")
	def := Definition{RuntimeEngine: "graph"}
	if !TeamGraphRuntimeEnabled(def) {
		t.Fatal("graph engine should enable graph runtime by default")
	}
	defEmpty := Definition{}
	if !TeamGraphRuntimeEnabled(defEmpty) {
		t.Fatal("empty runtime_engine should default to graph path")
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

func TestSupportsTeamGraphRuntimeMode(t *testing.T) {
	if !SupportsTeamGraphRuntimeMode("sequential") {
		t.Fatal("sequential should be supported")
	}
	if SupportsTeamGraphRuntimeMode("unknown") {
		t.Fatal("unknown mode should not be supported")
	}
}
