package team

import (
	"testing"

	"aranea-agents/internal/biz"
)

// 归因映射必须以「实际执行的图」为唯一真相源：C1 全量物化后，linked 资产拓扑
// 可能与 def 二次派生（EnabledMembers+memberNodeID）漂移——0 基 sort_order、
// custom 编辑交换节点都会让 def 派生映射把 step 记到错误成员名下。
func TestBuildAttributionFromCompiledTeam_MatchesExecutedTopology(t *testing.T) {
	defJSON := `{"version":2,"mode":"sequential","members":[
		{"agent_id":"sys-id","role":"worker","name":"系统巡检","enabled":true,"sort_order":0},
		{"agent_id":"net-id","role":"worker","name":"网络巡检","enabled":true,"sort_order":1}]}`
	// 物化资产拓扑：member-1=系统巡检（entry），member-2=网络巡检（finish）。
	ct := &biz.CompiledTeam{GraphBuildConfig: biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "member-1", Type: "agent", AgentName: "ops_system_inspection"},
			{ID: "member-2", Type: "agent", AgentName: "ops_network_inspection"},
		},
	}}
	agentKeyFn := func(id string) string {
		switch id {
		case "sys-id":
			return "ops_system_inspection"
		case "net-id":
			return "ops_network_inspection"
		}
		return id
	}

	memberByNode, stepSortIndex, reg, ok := buildAttributionFromCompiledTeam(ct, defJSON, agentKeyFn)
	if !ok {
		t.Fatal("expected attribution from compiled team")
	}
	if m := memberByNode["member-1"]; m.AgentID != "sys-id" {
		t.Fatalf("member-1 → agent %q, want sys-id（实际执行者）", m.AgentID)
	}
	if m := memberByNode["member-2"]; m.AgentID != "net-id" {
		t.Fatalf("member-2 → agent %q, want net-id", m.AgentID)
	}
	if stepSortIndex["member-1"] != 0 || stepSortIndex["member-2"] != 1 {
		t.Fatalf("stepSortIndex=%v want member-1:0 member-2:1", stepSortIndex)
	}
	if e := reg.ByAgentID["sys-id"]; e.NodeID != "member-1" {
		t.Fatalf("obsReg sys-id → %s want member-1", e.NodeID)
	}
	if e := reg.ByAgentKey["ops_network_inspection"]; e.NodeID != "member-2" {
		t.Fatalf("obsReg net key → %s want member-2", e.NodeID)
	}
}

// custom 编辑把拓扑换成非 member-N 节点 ID 时，归因仍按执行图节点生效。
func TestBuildAttributionFromCompiledTeam_CustomNodeIDs(t *testing.T) {
	defJSON := `{"version":2,"mode":"sequential","members":[
		{"agent_id":"sys-id","name":"系统巡检","sort_order":1},
		{"agent_id":"net-id","name":"网络巡检","sort_order":2}]}`
	ct := &biz.CompiledTeam{GraphBuildConfig: biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "node_alpha", Type: "agent", AgentName: "ops_network_inspection"},
			{ID: "node_beta", Type: "agent", AgentName: "ops_system_inspection"},
		},
	}}
	agentKeyFn := func(id string) string {
		if id == "sys-id" {
			return "ops_system_inspection"
		}
		return "ops_network_inspection"
	}

	memberByNode, stepSortIndex, _, ok := buildAttributionFromCompiledTeam(ct, defJSON, agentKeyFn)
	if !ok {
		t.Fatal("expected attribution")
	}
	if m := memberByNode["node_alpha"]; m.AgentID != "net-id" {
		t.Fatalf("node_alpha → %q want net-id", m.AgentID)
	}
	if m := memberByNode["node_beta"]; m.AgentID != "sys-id" {
		t.Fatalf("node_beta → %q want sys-id", m.AgentID)
	}
	if stepSortIndex["node_alpha"] != 0 || stepSortIndex["node_beta"] != 1 {
		t.Fatalf("stepSortIndex=%v", stepSortIndex)
	}
}

// ct 为 nil / 无 agent 节点 / def 无成员时返回 ok=false，调用方回退 def 派生映射。
func TestBuildAttributionFromCompiledTeam_Fallback(t *testing.T) {
	if _, _, _, ok := buildAttributionFromCompiledTeam(nil, `{"members":[]}`, nil); ok {
		t.Fatal("nil ct should not produce attribution")
	}
	ct := &biz.CompiledTeam{GraphBuildConfig: biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{{ID: "fn-1", Type: "function", FuncRef: "x"}},
	}}
	defJSON := `{"members":[{"agent_id":"a","sort_order":1}]}`
	if _, _, _, ok := buildAttributionFromCompiledTeam(ct, defJSON, nil); ok {
		t.Fatal("no agent nodes should not produce attribution")
	}
}

// member 的 agent_key 与节点 AgentName 匹配不上时（如成员被移出团队），
// 该节点不进入归因映射（step 跳过），其余节点不受影响。
func TestBuildAttributionFromCompiledTeam_UnmatchedNodeSkipped(t *testing.T) {
	defJSON := `{"members":[{"agent_id":"sys-id","name":"系统巡检","sort_order":1}]}`
	ct := &biz.CompiledTeam{GraphBuildConfig: biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "member-1", Type: "agent", AgentName: "ops_system_inspection"},
			{ID: "member-2", Type: "agent", AgentName: "removed_agent"},
		},
	}}
	agentKeyFn := func(id string) string { return "ops_system_inspection" }

	memberByNode, stepSortIndex, _, ok := buildAttributionFromCompiledTeam(ct, defJSON, agentKeyFn)
	if !ok {
		t.Fatal("expected attribution for matched node")
	}
	if len(memberByNode) != 1 || memberByNode["member-1"].AgentID != "sys-id" {
		t.Fatalf("memberByNode=%v", memberByNode)
	}
	if stepSortIndex["member-1"] != 0 {
		t.Fatalf("stepSortIndex=%v want member-1:0", stepSortIndex)
	}
}
