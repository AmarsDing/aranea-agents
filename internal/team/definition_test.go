package team

import (
	"testing"
	"time"
)

func TestEnabledMembers_sortOrder(t *testing.T) {
	d := Definition{
		Members: []MemberDef{
			{AgentID: "c", Role: "worker", SortOrder: 30},
			{AgentID: "a", Role: "worker", SortOrder: 10},
			{AgentID: "b", Role: "worker", SortOrder: 20},
		},
	}
	got := EnabledMembers(d)
	if len(got) != 3 || got[0].AgentID != "a" || got[1].AgentID != "b" || got[2].AgentID != "c" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestEnabledMembers_sortOrderZeroPreservesSliceOrder(t *testing.T) {
	d := Definition{
		Members: []MemberDef{
			{AgentID: "first", Role: "worker"},
			{AgentID: "second", Role: "worker"},
		},
	}
	got := EnabledMembers(d)
	if len(got) != 2 || got[0].AgentID != "first" || got[1].AgentID != "second" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestTurnDeadlineDuration_clamped(t *testing.T) {
	if got := TurnDeadlineDuration(Definition{TimeoutSeconds: 30}); got != 120*time.Second {
		t.Fatalf("got %v want 120s", got)
	}
	if got := TurnDeadlineDuration(Definition{TimeoutSeconds: 9000}); got != 7200*time.Second {
		t.Fatalf("got %v want 7200s", got)
	}
	if got := TurnDeadlineDuration(Definition{TimeoutSeconds: 0}); got != 0 {
		t.Fatalf("got %v want 0", got)
	}
}

func TestParseDefinition_normalizesZeroBasedSortOrder(t *testing.T) {
	// 前端团队编辑器以 0 基稠密 sort_order 保存（声明顺序即执行顺序）。
	// 解析边界统一归一化为 1 基稠密，使 EnabledMembers / memberNodeID /
	// 物化器 / observatory 注册表对同一 def 产生一致的 member-N 映射。
	raw := `{"version":2,"mode":"sequential","members":[
		{"agent_id":"sys","role":"worker","name":"系统巡检","enabled":true,"sort_order":0},
		{"agent_id":"net","role":"worker","name":"网络巡检","enabled":true,"sort_order":1}]}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if def.Members[0].SortOrder != 1 || def.Members[1].SortOrder != 2 {
		t.Fatalf("sort_order=%d,%d want 1,2", def.Members[0].SortOrder, def.Members[1].SortOrder)
	}
	// 归一化后有效顺序保持声明顺序，node ID 与之一致：sys→member-1, net→member-2
	members := EnabledMembers(def)
	if len(members) != 2 || members[0].AgentID != "sys" || members[1].AgentID != "net" {
		t.Fatalf("effective order wrong: %#v", members)
	}
	if id := memberNodeID(members[0], 0); id != "member-1" {
		t.Fatalf("node0=%s want member-1", id)
	}
	if id := memberNodeID(members[1], 1); id != "member-2" {
		t.Fatalf("node1=%s want member-2", id)
	}
}

func TestParseDefinition_sparsePositiveSortOrderUntouched(t *testing.T) {
	// 存量稀疏 1 基数据（10,20,30）保持原值：node ID member-10/member-20
	// 在各路径一致使用，不做稠密化以避免既有快照/资产漂移。
	raw := `{"mode":"sequential","members":[
		{"agent_id":"a","sort_order":10},{"agent_id":"b","sort_order":20}]}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if def.Members[0].SortOrder != 10 || def.Members[1].SortOrder != 20 {
		t.Fatalf("sort_order=%d,%d want untouched 10,20", def.Members[0].SortOrder, def.Members[1].SortOrder)
	}
}

func TestParseDefinition_duplicateSortOrderRemapped(t *testing.T) {
	// 重复值无法构成稳定排序键，按声明顺序重编为稠密 1 基。
	raw := `{"mode":"sequential","members":[
		{"agent_id":"a","sort_order":1},{"agent_id":"b","sort_order":1}]}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if def.Members[0].SortOrder != 1 || def.Members[1].SortOrder != 2 {
		t.Fatalf("sort_order=%d,%d want 1,2", def.Members[0].SortOrder, def.Members[1].SortOrder)
	}
}

func TestParseDefinition_explicitReorderRespected(t *testing.T) {
	// sort_order 是排序键：声明顺序与数值顺序冲突时以数值为有效顺序；
	// 值本身全正且无重复时不重写。
	raw := `{"mode":"sequential","members":[
		{"agent_id":"a","sort_order":2},{"agent_id":"b","sort_order":1}]}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if def.Members[0].SortOrder != 2 || def.Members[1].SortOrder != 1 {
		t.Fatalf("sort_order=%d,%d want 2,1", def.Members[0].SortOrder, def.Members[1].SortOrder)
	}
	members := EnabledMembers(def)
	if len(members) != 2 || members[0].AgentID != "b" || members[1].AgentID != "a" {
		t.Fatalf("effective order wrong: %#v", members)
	}
}

func TestParseDefinition_intentAnchorAgentID(t *testing.T) {
	raw := `{"version":1,"mode":"sequential","intent_anchor_agent_id":"agent-uuid-1","members":[]}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if def.IntentAnchorAgentID != "agent-uuid-1" {
		t.Fatalf("got %q", def.IntentAnchorAgentID)
	}
}

func TestParseDefinition_enableStateDeliverable(t *testing.T) {
	// Explicitly enabled
	raw := `{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[]}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !def.EnableStateDeliverable {
		t.Fatal("expected EnableStateDeliverable=true when JSON sets it")
	}

	// Default false when omitted
	def2, err := ParseDefinition(`{"version":1,"mode":"sequential","members":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if def2.EnableStateDeliverable {
		t.Fatal("expected EnableStateDeliverable=false by default")
	}

	// Explicitly false
	def3, err := ParseDefinition(`{"version":1,"mode":"sequential","enable_state_deliverable":false,"members":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if def3.EnableStateDeliverable {
		t.Fatal("expected EnableStateDeliverable=false when JSON sets it false")
	}
}
