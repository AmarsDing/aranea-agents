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

func TestTeamSummaryActivityEvent(t *testing.T) {
	// SessionID must be the spirit session ID (not run.SessionID which is the
	// team session ID) so the frontend WS filter and listActivities API return
	// this team_stage summary event. See publishSpiritTeamAssembled canonical setup.
	run := biz.TeamRunRecord{ID: "r1", TeamID: "t1", SessionID: "s1", SpiritSessionID: "spirit-1", Status: biz.TeamRunStatusSuccess}
	ev := TeamSummaryActivityEvent(run, nil)
	if ev.Event != biz.ActivityEventCompleted {
		t.Fatalf("event=%s want=%s", ev.Event, biz.ActivityEventCompleted)
	}
	if ev.Activity.Kind != biz.ActivityKindTeamStage {
		t.Fatalf("kind=%s want=%s", ev.Activity.Kind, biz.ActivityKindTeamStage)
	}
	if ev.Activity.Status != biz.ActivityStatusCompleted {
		t.Fatalf("status=%s want=%s", ev.Activity.Status, biz.ActivityStatusCompleted)
	}
	if ev.Activity.Stage != "completed" {
		t.Fatalf("stage=%s want=completed", ev.Activity.Stage)
	}
	if ev.Domain != biz.ActivityDomainChat {
		t.Fatalf("domain=%s want=%s", ev.Domain, biz.ActivityDomainChat)
	}
	if ev.Activity.SessionID != "spirit-1" {
		t.Fatalf("session_id=%s want=spirit-1", ev.Activity.SessionID)
	}
	if ev.Activity.SpiritSessionID != "spirit-1" {
		t.Fatalf("spirit_session_id=%s want=spirit-1", ev.Activity.SpiritSessionID)
	}
	if ev.Activity.TeamID != "t1" {
		t.Fatalf("team_id=%s want=t1", ev.Activity.TeamID)
	}
	if ev.Activity.Meta["team_summary"] == nil {
		t.Fatal("missing team_summary in meta")
	}
	if ev.Activity.Meta["run_id"] != "r1" {
		t.Fatalf("meta.run_id=%v want=r1", ev.Activity.Meta["run_id"])
	}
}
