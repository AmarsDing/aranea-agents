package team

import "testing"

func TestTeamGraphRuntimeEnabledForTeam_delegates(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "1")
	t.Setenv("ARANEA_TEAM_NATIVE", "")
	def := Definition{Mode: "sequential"}
	if !TeamGraphRuntimeEnabledForTeam(def, "any-team") {
		t.Fatal("should delegate to TeamGraphRuntimeEnabled")
	}
}

func TestTeamGraphRuntimeEnabledForTeam_nativeForced(t *testing.T) {
	t.Setenv("ARANEA_TEAM_GRAPH_RUNTIME", "1")
	t.Setenv("ARANEA_TEAM_NATIVE", "1")
	def := Definition{Mode: "sequential"}
	if TeamGraphRuntimeEnabledForTeam(def, "any-team") {
		t.Fatal("native forced should disable graph")
	}
}

func TestDecideNativeFallback_emergency(t *testing.T) {
	t.Setenv("ARANEA_TEAM_NATIVE", "1")
	d := DecideNativeFallback(Definition{}, "t1", false, "", "", "sequential", false)
	if !d.UseNative {
		t.Fatal("native forced should use native")
	}
	if d.MetricLabel != "native_emergency" {
		t.Fatalf("metric=%s want native_emergency", d.MetricLabel)
	}
}

func TestDecideNativeFallback_graphFail(t *testing.T) {
	t.Setenv("ARANEA_TEAM_NATIVE", "")
	d := DecideNativeFallback(Definition{}, "t1", true, "compile err", "", "sequential", true)
	if d.UseNative {
		t.Fatal("should not use native when not forced")
	}
	if d.ErrorMessage == "" {
		t.Fatal("should have diagnostic error message")
	}
}
