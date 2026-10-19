package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"context"
	"testing"
)

func TestAgentAllocator_MatchSubTask_NoLowScoreAssign(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{
		AgentKey: "be", DisplayName: "后端", Roles: []string{"backend"},
	}}
	_, err := impl.matchSubTask(context.Background(), biz.SubTask{
		ID: "st1", Name: "文案", RequiredCapabilities: []string{"unrelated-xyz"},
	}, caps, "tr")
	if err == nil {
		t.Fatal("below-threshold overlap must be a miss")
	}
}

func TestAgentAllocator_AllocateExplicit_Subtasks(t *testing.T) {
	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "__system_admin__", DisplayName: "系统管家", Status: "active"},
	}}
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		lg:          loggateway.NewNoop(),
	}

	plan := &biz.TaskPlan{
		ID:              "tp_explicit",
		SpiritSessionID: "sess-explicit",
		TraceID:         "trace-explicit",
		SubTasks: []biz.SubTask{
			{ID: "st_1", Name: "安装 Skill A"},
			{ID: "st_2", Name: "安装 Skill B"},
		},
		Strategy: biz.StrategyParallel,
	}

	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"__system_admin__"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	if len(saved.Allocations) != 2 {
		t.Fatalf("allocations count=%d want 2", len(saved.Allocations))
	}
	for i, alloc := range saved.Allocations {
		if alloc.AssignedKey != "__system_admin__" {
			t.Errorf("allocations[%d].AssignedKey=%q want __system_admin__", i, alloc.AssignedKey)
		}
		if alloc.AssignedName != "系统管家" {
			t.Errorf("allocations[%d].AssignedName=%q want 系统管家", i, alloc.AssignedName)
		}
		if alloc.MatchLayer != "explicit" {
			t.Errorf("allocations[%d].MatchLayer=%q want explicit", i, alloc.MatchLayer)
		}
		if alloc.AssignedType != "agent" {
			t.Errorf("allocations[%d].AssignedType=%q want agent", i, alloc.AssignedType)
		}
	}
	if saved.Allocations[0].SubTaskID != "st_1" || saved.Allocations[1].SubTaskID != "st_2" {
		t.Errorf("order not preserved: %s, %s", saved.Allocations[0].SubTaskID, saved.Allocations[1].SubTaskID)
	}
	if repo.saved == nil {
		t.Fatal("AllocateExplicit did not persist the allocation plan")
	}
}

func TestAgentAllocator_AllocateExplicit_DagTeamMembers(t *testing.T) {
	reader := &stubAgentReader{agents: nil}
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		lg:          loggateway.NewNoop(),
	}

	plan := &biz.TaskPlan{
		ID:              "tp_explicit_dag",
		SpiritSessionID: "sess-explicit-dag",
		SubTasks:        []biz.SubTask{{ID: "st_1", Name: "协作任务"}},
		Strategy:        biz.StrategyDAG,
	}

	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"lead-x", "member-y", "member-z"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	if len(saved.Allocations) != 1 {
		t.Fatalf("allocations count=%d want 1", len(saved.Allocations))
	}
	alloc := saved.Allocations[0]
	if alloc.AssignedKey != "lead-x" {
		t.Errorf("AssignedKey=%q want lead-x", alloc.AssignedKey)
	}
	if alloc.AssignedType != "team" {
		t.Errorf("AssignedType=%q want team (dag + multiple keys)", alloc.AssignedType)
	}
	if len(alloc.TeamMemberKeys) != 2 || alloc.TeamMemberKeys[0] != "member-y" || alloc.TeamMemberKeys[1] != "member-z" {
		t.Errorf("TeamMemberKeys=%v want [member-y member-z]", alloc.TeamMemberKeys)
	}
}

func TestAgentAllocator_AllocateExplicit_WholePlan(t *testing.T) {
	reader := &stubAgentReader{agents: nil}
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		lg:          loggateway.NewNoop(),
	}

	plan := &biz.TaskPlan{
		ID:              "tp_explicit_whole",
		SpiritSessionID: "sess-explicit-whole",
		SubTasks:        nil,
		Strategy:        biz.StrategyParallel,
	}

	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"__memory__"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	if len(saved.Allocations) != 1 {
		t.Fatalf("allocations count=%d want 1", len(saved.Allocations))
	}
	if saved.Allocations[0].SubTaskID != "whole" {
		t.Errorf("SubTaskID=%q want whole", saved.Allocations[0].SubTaskID)
	}
	if saved.Allocations[0].AssignedKey != "__memory__" {
		t.Errorf("AssignedKey=%q want __memory__", saved.Allocations[0].AssignedKey)
	}
}

func TestAgentAllocator_AllocateExplicit_Validation(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	if _, err := impl.AllocateExplicit(context.Background(), nil, []string{"a"}); err == nil {
		t.Fatal("nil plan: want error")
	}
	plan := &biz.TaskPlan{ID: "tp_v", SpiritSessionID: "s"}
	if _, err := impl.AllocateExplicit(context.Background(), plan, nil); err == nil {
		t.Fatal("empty agent_keys: want error")
	}
	if _, err := impl.AllocateExplicit(context.Background(), plan, []string{"  ", ""}); err == nil {
		t.Fatal("blank agent_keys: want error")
	}
}

// P2-1 (session-eval-20260829-r2 R4-Q1): Spirit LLM 把中文部门名当 agent_keys
// 传入时必须解析为真实 agent_key，否则下游 assembly 硬失败 → 零 team_runs。
func TestAgentAllocator_AllocateExplicit_ResolvesDepartmentNames(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: biz.DeptLeadAgentKeyPrefix + "eng__", DisplayName: "研发主管", Status: "active", AgentVariant: "dept_lead", PositionID: "dept-eng"},
		{AgentKey: "be", DisplayName: "后端工程师", Status: "active", PositionID: "pos-be", Roles: []string{"backend"}},
		{AgentKey: "copy", DisplayName: "文案专员", Status: "active", PositionID: "pos-copy", Roles: []string{"copywriter"}},
	}
	reader := &stubAgentReader{agents: agents}
	capBuilder := NewAgentCapabilityBuilder(reader, loggateway.NewNoop())
	capBuilder.SetOrganizationReader(&stubOrgReader{nodes: sampleOrgTree()})
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		capBuilder:  capBuilder,
		lg:          loggateway.NewNoop(),
	}

	plan := &biz.TaskPlan{
		ID:              "tp_explicit_dept",
		SpiritSessionID: "sess-explicit-dept",
		SubTasks:        []biz.SubTask{{ID: "st_1", Name: "技术侧发布计划"}, {ID: "st_2", Name: "内容侧宣传文案"}},
		Strategy:        biz.StrategyParallel,
	}

	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"研发部", "内容运营部"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	if len(saved.Allocations) != 2 {
		t.Fatalf("allocations count=%d want 2", len(saved.Allocations))
	}

	if got := saved.Allocations[0].AssignedKey; got != biz.DeptLeadAgentKeyPrefix+"eng__" {
		t.Errorf("allocations[0].AssignedKey=%q want dept lead eng", got)
	}
	if got := saved.Allocations[1].AssignedKey; got != "copy" {
		t.Errorf("allocations[1].AssignedKey=%q want copy (dept member fallback)", got)
	}
}

// P2-1: 无法解析的 key 保持原样透传（既有契约不回归）。
func TestAgentAllocator_AllocateExplicit_UnresolvableKeptVerbatim(t *testing.T) {
	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "be", DisplayName: "后端工程师", Status: "active"},
	}}
	capBuilder := NewAgentCapabilityBuilder(reader, loggateway.NewNoop())
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		capBuilder:  capBuilder,
		lg:          loggateway.NewNoop(),
	}
	plan := &biz.TaskPlan{
		ID:              "tp_explicit_unknown",
		SpiritSessionID: "sess-explicit-unknown",
		SubTasks:        []biz.SubTask{{ID: "st_1", Name: "任务"}},
		Strategy:        biz.StrategyParallel,
	}
	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"不存在的部门"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	if got := saved.Allocations[0].AssignedKey; got != "不存在的部门" {
		t.Errorf("AssignedKey=%q want verbatim passthrough", got)
	}
}

// P2-1: parallel 模式且 agent_keys 数量与子任务一致时逐个子任务指派。
func TestAgentAllocator_AllocateExplicit_PerSubtaskMapping(t *testing.T) {
	reader := &stubAgentReader{agents: nil}
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		lg:          loggateway.NewNoop(),
	}
	plan := &biz.TaskPlan{
		ID:              "tp_explicit_persub",
		SpiritSessionID: "sess-explicit-persub",
		SubTasks: []biz.SubTask{
			{ID: "st_1", Name: "技术侧"},
			{ID: "st_2", Name: "内容侧"},
			{ID: "st_3", Name: "运营侧"},
		},
		Strategy: biz.StrategyParallel,
	}
	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"k-tech", "k-content", "k-ops"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	want := []string{"k-tech", "k-content", "k-ops"}
	for i, alloc := range saved.Allocations {
		if alloc.AssignedKey != want[i] {
			t.Errorf("allocations[%d].AssignedKey=%q want %q", i, alloc.AssignedKey, want[i])
		}
	}
}
