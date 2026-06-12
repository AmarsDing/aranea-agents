package team

import "testing"

func TestTeamGraphRuntimeEnabledForTeam_delegates(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "1")
	def := Definition{Mode: "sequential"}
	if !TeamGraphRuntimeEnabledForTeam(def, "any-team") {
		t.Fatal("should delegate to TeamGraphRuntimeEnabled")
	}
}
