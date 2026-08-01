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

func TestBuildOrchestrationRegistryFromDefinition_ZeroBasedSortOrderNoCollision(t *testing.T) {
	// 前端 0 基稠密 sort_order（0,1）：归一化后成员各占独立节点，
	// 不得因 ≤0→i+1 兜底双双坍缩到 member-1（曾导致 observatory 只显示一个节点）。
	def := `{"version":2,"mode":"sequential","members":[{"agent_id":"sys","role":"worker","sort_order":0,"name":"系统巡检"},{"agent_id":"net","role":"worker","sort_order":1,"name":"网络巡检"}]}`
	reg := BuildOrchestrationRegistryFromDefinition(def, nil)
	if len(reg.ByNodeID) != 2 {
		t.Fatalf("nodes: %d want 2 (collision?)", len(reg.ByNodeID))
	}
	if e := reg.ByAgentID["sys"]; e.NodeID != "member-1" {
		t.Fatalf("sys → %s want member-1", e.NodeID)
	}
	if e := reg.ByAgentID["net"]; e.NodeID != "member-2" {
		t.Fatalf("net → %s want member-2", e.NodeID)
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
