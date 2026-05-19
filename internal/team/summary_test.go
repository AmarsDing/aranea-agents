package team

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

func TestBuildTeamRunSummary(t *testing.T) {
	run := biz.TeamRun{
		ID: "run-1", TeamID: "team-1", SessionID: "sess-1",
		Mode: "sequential", Status: "success",
		TokenIn: 10, TokenOut: 20, DurationMS: 100,
	}
	steps := []biz.TeamRunStep{
		{AgentKey: "a1", AgentName: "Agent One", Role: "worker", SortOrder: 0, Status: "ok", TokenOut: 20},
	}
	summary := BuildTeamRunSummary(run, steps)
	if summary["run_id"] != "run-1" {
		t.Fatalf("run_id=%v", summary["run_id"])
	}
	members, ok := summary["members"].([]map[string]any)
	if !ok || len(members) != 1 {
		t.Fatalf("members=%T %#v", summary["members"], summary["members"])
	}
	if members[0]["agent_key"] != "a1" {
		t.Fatalf("agent_key=%v", members[0]["agent_key"])
	}
}

func TestTeamSummaryEnvelope(t *testing.T) {
	run := biz.TeamRun{ID: "r1", TeamID: "t1", SessionID: "s1", Status: "success"}
	env := TeamSummaryEnvelope(run, nil)
	if env.Type != event.EnvelopeTypeTeamSummary {
		t.Fatalf("type=%s", env.Type)
	}
	meta := env.Metadata
	if meta["team_summary"] == nil {
		t.Fatal("missing team_summary")
	}
}
