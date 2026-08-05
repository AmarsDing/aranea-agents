package team

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestBuildTeamRunSummary(t *testing.T) {
	run := biz.TeamRunRecord{
		ID: "run-1", TeamID: "team-1", SessionID: "sess-1",
		Mode: "sequential", Status: biz.TeamRunStatusSuccess,
		TokenIn: 10, TokenOut: 20, DurationMS: 100,
	}
	steps := []biz.TeamRunStep{
		{AgentKey: "a1", AgentName: "Agent One", Role: "worker", SortOrder: 0, Status: biz.TeamMemberStepStatusOK, TokenOut: 20, ToolCallCount: 2},
	}
	summary := BuildTeamRunSummary(run, steps)
	if summary["run_id"] != "run-1" {
		t.Fatalf("run_id=%v", summary["run_id"])
	}
	if summary["tool_call_count"] != 2 {
		t.Fatalf("tool_call_count=%v", summary["tool_call_count"])
	}
	members, ok := summary["members"].([]map[string]any)
	if !ok || len(members) != 1 {
		t.Fatalf("members=%T %#v", summary["members"], summary["members"])
	}
	if members[0]["agent_key"] != "a1" {
		t.Fatalf("agent_key=%v", members[0]["agent_key"])
	}
}

func TestSummaryMapFromDataMatchesBuildTeamRunSummary(t *testing.T) {
	run := biz.TeamRunRecord{ID: "run-1", TeamID: "t1", SessionID: "s1", Status: biz.TeamRunStatusSuccess}
	steps := []biz.TeamRunStep{{AgentKey: "a1", ToolCallCount: 1}}
	data := biz.BuildTeamRunSummaryData(run, steps)
	if got := SummaryMapFromData(data); got["run_id"] != BuildTeamRunSummary(run, steps)["run_id"] {
		t.Fatalf("map mismatch: %+v vs %+v", got, BuildTeamRunSummary(run, steps))
	}
}
