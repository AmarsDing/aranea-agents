package agent

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func TestBindRosterSpecialist_PrimaryAndBackup(t *testing.T) {
	pool := []biz.AgentCapability{
		{AgentKey: "copy-b", DisplayName: "文案B", DomainPath: "创作/文案"},
		{AgentKey: "copy-a", DisplayName: "文案A", DomainPath: "创作/文案"},
		{AgentKey: biz.DeptLeadAgentKeyPrefix + "media__", DisplayName: "主管", AgentVariant: "dept_lead", DomainPath: "创作/文案"},
		{AgentKey: biz.CompanyLeadAgentKeyPrefix + "acme__", DisplayName: "总经理", AgentVariant: biz.AgentVariantCompanyLead, DomainPath: "创作/文案"},
	}
	got, backup, ok := bindRosterSpecialist("创作/文案", "", pool)
	if !ok || got.AgentKey != "copy-a" {
		t.Fatalf("primary=%q ok=%v", got.AgentKey, ok)
	}
	if backup != "copy-b" {
		t.Fatalf("backup=%q", backup)
	}
}

func TestBindRosterSpecialist_RoleAlias(t *testing.T) {
	pool := []biz.AgentCapability{
		{AgentKey: "cw", DisplayName: "文案", Roles: []string{"copy"}},
	}
	got, _, ok := bindRosterSpecialist("创作/文案", "", pool)
	if !ok || got.AgentKey != "cw" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestBindRosterSpecialist_OtherIsMiss(t *testing.T) {
	_, _, ok := bindRosterSpecialist("其他", "", []biz.AgentCapability{
		{AgentKey: "x", DomainPath: "其他"},
	})
	if ok {
		t.Fatal("其他 must not bind")
	}
}

// TestBindRosterSpecialist_GenericRoleMapping 钉住包B B3a
// （session-eval-20260825, P-ROSTER-GAP）：planner LLM 按行业常识产出的
// 通用角色词（技术/内容/运营）归一化落「其他」，词表匹配失败后走通用角色
// 映射兜底到一级域，不再直接 roster miss（S07-A BAD_REQUEST）。
func TestBindRosterSpecialist_GenericRoleMapping(t *testing.T) {
	pool := []biz.AgentCapability{
		{AgentKey: "be", DisplayName: "后端工程师", DomainPath: "软件/后端"},
		{AgentKey: "cw", DisplayName: "文案", DomainPath: "创作/文案"},
		{AgentKey: "ops", DisplayName: "客服运营", DomainPath: "商务/客服"},
	}
	tests := []struct {
		name       string
		domainPath string
		taskText   string
		wantKey    string
	}{
		{"技术→软件一级域", "技术", "", "be"},
		{"研发→软件一级域", "研发", "", "be"},
		{"内容→创作一级域", "内容", "", "cw"},
		{"运营→商务一级域", "运营", "", "ops"},
		{"domainPath 无命中回退任务文本", "", "接口开发联调", "be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, ok := bindRosterSpecialist(tt.domainPath, tt.taskText, pool)
			if !ok || got.AgentKey != tt.wantKey {
				t.Fatalf("domainPath=%q taskText=%q: got key=%q ok=%v, want %q",
					tt.domainPath, tt.taskText, got.AgentKey, ok, tt.wantKey)
			}
		})
	}

	// 词表内精确路径不受兜底影响（优先精确匹配）。
	got, _, ok := bindRosterSpecialist("创作/文案", "", pool)
	if !ok || got.AgentKey != "cw" {
		t.Fatalf("lexicon path must keep exact binding, got %q ok=%v", got.AgentKey, ok)
	}
	// 完全无法归类的仍然 miss（保持 roster miss 语义给 B3b 降级）。
	if _, _, ok := bindRosterSpecialist("占星", "", pool); ok {
		t.Fatal("unclassifiable specialty must still miss")
	}
}

// TestRosterMissError_Sentinel 钉住包B B3b：rosterMissError 必须挂载
// biz.ErrRosterMiss 哨兵（errors.Is 可达），plan_and_execute 才能捕获并
// 降级为结构化 NextAction=build_orchestration_graph。
func TestRosterMissError_Sentinel(t *testing.T) {
	err := rosterMissError(biz.SubTask{ID: "st1", Name: "技术开发", DomainPath: "技术"}, nil)
	if err == nil {
		t.Fatal("expect error")
	}
	if !errors.Is(err, biz.ErrRosterMiss) {
		t.Fatalf("rosterMissError must wrap biz.ErrRosterMiss, got %v", err)
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("must stay BAD_REQUEST for WS failure event compat, got %v", err)
	}
}

func TestAllocate_RosterBindSkipsFactory(t *testing.T) {
	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "cw", DisplayName: "文案", Status: "active", DomainPath: "创作/文案"},
		{AgentKey: "be", DisplayName: "后端", Status: "active", DomainPath: "软件/后端"},
	}}
	factory := &fakeAllocatorAgentFactory{agentKey: "factory-x"}
	impl := &agentAllocatorImpl{
		repo:         &fakeAllocatorRepo{},
		agentReader:  reader,
		capBuilder:   NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		agentFactory: factory,
		lg:           loggateway.NewNoop(),
	}
	saved, err := impl.Allocate(context.Background(), &biz.TaskPlan{
		ID:       "tp-roster",
		Strategy: biz.StrategyDAG,
		SubTasks: []biz.SubTask{
			{ID: "st1", Name: "种草", DomainPath: "创作/文案"},
			{ID: "st2", Name: "接口", DomainPath: "软件/后端"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Allocations[0].AssignedKey != "cw" || saved.Allocations[0].MatchLayer != "roster" {
		t.Fatalf("st1 %+v", saved.Allocations[0])
	}
	if saved.Allocations[1].AssignedKey != "be" || saved.Allocations[1].MatchLayer != "roster" {
		t.Fatalf("st2 %+v", saved.Allocations[1])
	}
	if len(factory.profiles) != 0 {
		t.Fatalf("factory must stay idle, calls=%d", len(factory.profiles))
	}
}

func TestAllocate_PublishesCollaboratingAndStaffingMeta(t *testing.T) {
	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "cw", DisplayName: "文案专项", Status: "active", DomainPath: "创作/文案"},
		{AgentKey: "de", DisplayName: "视觉专项", Status: "active", DomainPath: "设计/视觉"},
	}}
	bus := &allocatorCaptureBus{}
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:          loggateway.NewNoop(),
		bus:         bus,
	}
	_, err := impl.Allocate(context.Background(), &biz.TaskPlan{
		ID:              "tp-collab",
		SpiritSessionID: "sp-1",
		Strategy:        biz.StrategyDAG,
		SubTasks: []biz.SubTask{
			{ID: "st1", Name: "种草", DomainPath: "创作/文案"},
			{ID: "st2", Name: "主视觉", DomainPath: "设计/视觉", DependsOn: []string{"st1"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawCollab, sawBound bool
	for _, e := range bus.getPublished() {
		n, ok := e.(*biz.SystemNoticeEvent)
		if !ok || n.NoticeType != "orchestration_progress" {
			continue
		}
		if n.Meta["phase"] == "collaborating" {
			sawCollab = true
			if n.Meta["unifier"] != unifierBriefAndSpirit {
				t.Fatalf("unifier=%v", n.Meta["unifier"])
			}
		}
		if n.Meta["phase"] == "allocating" && n.Meta["match_layer"] == "roster" && n.Meta["specialty"] == "创作/文案" {
			sawBound = true
		}
	}
	if !sawCollab || !sawBound {
		t.Fatalf("collab=%v bound=%v notices=%d", sawCollab, sawBound, len(bus.getPublished()))
	}
}

func TestCollectSpecialtySlots_PreservesOrder(t *testing.T) {
	got := collectSpecialtySlots([]biz.SubTask{
		{DomainPath: "创作/文案"},
		{DomainPath: "创作/文案"},
		{DomainPath: "设计/视觉"},
		{DomainPath: "其他"},
	})
	if len(got) != 2 || got[0] != "创作/文案" || got[1] != "设计/视觉" {
		t.Fatalf("%v", got)
	}
}

func TestRecipeReplaySubTasks_FromSpecialties(t *testing.T) {
	got := recipeReplaySubTasks(&biz.MemoryHit{
		DomainPath:  "创作",
		Specialties: []string{"创作/文案", "设计/视觉"},
	}, "做一套种草物料")
	if len(got) != 2 || got[0].ID != "st_recipe_1" || got[1].DomainPath != "设计/视觉" {
		t.Fatalf("%+v", got)
	}
}
