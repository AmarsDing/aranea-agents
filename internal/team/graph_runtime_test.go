package team

import (
	"os"
	"testing"
)

func TestTeamGraphRuntimeEnabled(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "1")
	def := Definition{RuntimeEngine: "graph"}
	if !TeamGraphRuntimeEnabled(def) {
		t.Fatal("expected enabled")
	}
	def2 := Definition{RuntimeEngine: "native"}
	if TeamGraphRuntimeEnabled(def2) {
		t.Fatal("expected disabled without flag in definition")
	}
	def3 := Definition{TeamGraphRuntime: true}
	if !TeamGraphRuntimeEnabled(def3) {
		t.Fatal("expected enabled via team_graph_runtime")
	}
}

func TestSupportsTeamGraphRuntimeMode(t *testing.T) {
	if !SupportsTeamGraphRuntimeMode("sequential") {
		t.Fatal("sequential should be supported")
	}
	if !SupportsTeamGraphRuntimeMode("adaptive") {
		t.Fatal("adaptive should be supported")
	}
	if !SupportsTeamGraphRuntimeMode("swarm") {
		t.Fatal("swarm should be supported")
	}
}

func TestEnvTeamGraphRuntimeGate(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "")
	if envTeamGraphRuntimeGate() {
		t.Fatal("expected off")
	}
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "true")
	if !envTeamGraphRuntimeGate() {
		t.Fatal("expected on")
	}
	_ = os.Getenv("ARANEA_TEAM_GRAPH_RUNTIME")
}
