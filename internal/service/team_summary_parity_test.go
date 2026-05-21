package service

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
)

func TestRunSummaryMapProtoParity(t *testing.T) {
	run := biz.TeamRun{
		ID: "run-1", TeamID: "team-1", SessionID: "sess-1",
		Mode: "sequential", Status: "success",
		TokenIn: 10, TokenOut: 20, DurationMS: 100, CostMicroUSD: 42,
		OutputPreview: "team output", ErrorMessage: "  ",
	}
	steps := []biz.TeamRunStep{
		{
			AgentID: "ag-1", AgentKey: "a1", AgentName: "Agent One", Role: "worker",
			SortOrder: 0, Status: "ok", TokenIn: 3, TokenOut: 20, DurationMS: 50,
			CostMicroUSD: 21, ToolCallCount: 2, OutputPreview: "member output",
		},
	}
	data := biz.BuildTeamRunSummaryData(run, steps)
	m := team.SummaryMapFromData(data)
	p := toProtoTeamRunSummary(data)

	assertIntField(t, "member_count", m["member_count"], int(p.GetMemberCount()))
	assertIntField(t, "tool_call_count", m["tool_call_count"], int(p.GetToolCallCount()))
	assertStringField(t, "run_id", m["run_id"], p.GetRunId())
	assertStringField(t, "team_id", m["team_id"], p.GetTeamId())
	assertStringField(t, "session_id", m["session_id"], p.GetSessionId())
	assertStringField(t, "mode", m["mode"], p.GetMode())
	assertStringField(t, "status", m["status"], p.GetStatus())
	assertIntField(t, "duration_ms", m["duration_ms"], int(p.GetDurationMs()))
	assertIntField(t, "token_in", m["token_in"], int(p.GetTokenIn()))
	assertIntField(t, "token_out", m["token_out"], int(p.GetTokenOut()))
	assertInt64Field(t, "cost_micro_usd", m["cost_micro_usd"], p.GetCostMicroUsd())
	assertStringField(t, "output_preview", m["output_preview"], p.GetOutputPreview())
	assertStringField(t, "error_message", m["error_message"], p.GetErrorMessage())

	members, ok := m["members"].([]map[string]any)
	if !ok {
		t.Fatalf("members type=%T", m["members"])
	}
	if len(members) != len(p.GetMembers()) {
		t.Fatalf("member count map=%d proto=%d", len(members), len(p.GetMembers()))
	}
	pm := p.GetMembers()[0]
	mm := members[0]
	assertStringField(t, "member.agent_id", mm["agent_id"], pm.GetAgentId())
	assertStringField(t, "member.agent_key", mm["agent_key"], pm.GetAgentKey())
	assertStringField(t, "member.agent_name", mm["agent_name"], pm.GetAgentName())
	assertStringField(t, "member.role", mm["role"], pm.GetRole())
	assertIntField(t, "member.sort_order", mm["sort_order"], int(pm.GetSortOrder()))
	assertStringField(t, "member.status", mm["status"], pm.GetStatus())
	assertIntField(t, "member.token_in", mm["token_in"], int(pm.GetTokenIn()))
	assertIntField(t, "member.token_out", mm["token_out"], int(pm.GetTokenOut()))
	assertIntField(t, "member.duration_ms", mm["duration_ms"], int(pm.GetDurationMs()))
	assertInt64Field(t, "member.cost_micro_usd", mm["cost_micro_usd"], pm.GetCostMicroUsd())
	assertStringField(t, "member.output_preview", mm["output_preview"], pm.GetOutputPreview())
	assertIntField(t, "member.tool_call_count", mm["tool_call_count"], int(pm.GetToolCallCount()))
}

func assertStringField(t *testing.T, name string, got any, want string) {
	t.Helper()
	s, ok := got.(string)
	if !ok || s != want {
		t.Fatalf("%s: got=%#v want=%q", name, got, want)
	}
}

func assertIntField(t *testing.T, name string, got any, want int) {
	t.Helper()
	n, ok := got.(int)
	if !ok || n != want {
		t.Fatalf("%s: got=%#v want=%d", name, got, want)
	}
}

func assertInt64Field(t *testing.T, name string, got any, want int64) {
	t.Helper()
	n, ok := got.(int64)
	if !ok || n != want {
		t.Fatalf("%s: got=%#v want=%d", name, got, want)
	}
}
