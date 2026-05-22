package biz

import "testing"

func TestResolveChannelAsyncGraphTarget_team(t *testing.T) {
	target, err := ResolveChannelAsyncGraphTarget(ChannelLongTaskConfig{AsyncTeamID: "team-1"})
	if err != nil {
		t.Fatal(err)
	}
	if target.TargetType != "team_graph" || target.TeamID != "team-1" {
		t.Fatalf("target=%+v", target)
	}
}

func TestResolveChannelAsyncGraphTarget_graph(t *testing.T) {
	target, err := ResolveChannelAsyncGraphTarget(ChannelLongTaskConfig{AsyncGraphID: "g-1"})
	if err != nil {
		t.Fatal(err)
	}
	if target.TargetType != "graph" || target.GraphID != "g-1" {
		t.Fatalf("target=%+v", target)
	}
}

func TestResolveChannelAsyncGraphTarget_teamPreferred(t *testing.T) {
	target, err := ResolveChannelAsyncGraphTarget(ChannelLongTaskConfig{AsyncTeamID: "t1", AsyncGraphID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	if target.TargetType != "team_graph" {
		t.Fatalf("target=%+v", target)
	}
}
