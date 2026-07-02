package biz

import "testing"

func TestBuildTeamRunObservatory_FromSteps(t *testing.T) {
	def := `{"version":1,"mode":"sequential","members":[{"agent_id":"a1","role":"worker","sort_order":10,"name":"A"},{"agent_id":"a2","role":"worker","sort_order":20,"name":"B"}]}`
	run := TeamRunRecord{ID: "run-1", TeamID: "t1", SessionID: "s1", Status: "success", Mode: "sequential"}
	steps := []TeamRunStep{
		{AgentID: "a1", AgentKey: "w1", AgentName: "Worker 1", SortOrder: 10, Status: "ok", OutputPreview: "out1"},
		{AgentID: "a2", AgentKey: "w2", AgentName: "Worker 2", SortOrder: 20, Status: "error", ErrorMessage: "boom"},
	}
	obs := BuildTeamRunObservatory(run, steps, def)
	if len(obs.Nodes) != 2 {
		t.Fatalf("nodes: %d", len(obs.Nodes))
	}
	byID := map[string]AgentNodeState{}
	for _, n := range obs.Nodes {
		byID[n.NodeID] = n
	}
	if byID["member-10"].Status != AgentNodeStatusSuccess {
		t.Fatalf("member-10: %s", byID["member-10"].Status)
	}
	if byID["member-20"].Status != AgentNodeStatusFailed {
		t.Fatalf("member-20: %s", byID["member-20"].Status)
	}
}

func TestHasActiveTeamRun(t *testing.T) {
	if !HasActiveTeamRun([]TeamRunRecord{{Status: "running"}}) {
		t.Fatal("expected active")
	}
	if HasActiveTeamRun([]TeamRunRecord{{Status: "success"}}) {
		t.Fatal("expected inactive")
	}
}
