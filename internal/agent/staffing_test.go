package agent

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type pickStaffingAdvisor struct {
	key string
	got []biz.StaffingAsk
}

func (p *pickStaffingAdvisor) Suggest(_ context.Context, in biz.StaffingAsk) (biz.StaffingReply, error) {
	p.got = append(p.got, in)
	return biz.StaffingReply{AgentKeys: []string{p.key}}, nil
}

type factoryStaffingAdvisor struct{}

func (factoryStaffingAdvisor) Suggest(context.Context, biz.StaffingAsk) (biz.StaffingReply, error) {
	return biz.StaffingReply{UseFactory: true}, nil
}

type blockUntilCancelAdvisor struct{}

func (blockUntilCancelAdvisor) Suggest(ctx context.Context, _ biz.StaffingAsk) (biz.StaffingReply, error) {
	<-ctx.Done()
	return biz.StaffingReply{}, ctx.Err()
}

func unmatchedCopyPlan() *biz.TaskPlan {
	return &biz.TaskPlan{
		ID:       "tp-staff",
		TraceID:  "tr-staff",
		Strategy: biz.StrategyDirect,
		SubTasks: []biz.SubTask{{
			ID:                   "st1",
			Name:                 "任务A",
			Description:          "xyz-unmatched-capability",
			RequiredCapabilities: []string{"quantum-review"},
			DomainPath:           "创作/文学",
		}},
	}
}

func staffingAllocator(t *testing.T, advisor biz.StaffingAdvisor, factory biz.AgentFactory) *agentAllocatorImpl {
	t.Helper()
	agents := []biz.Agent{
		{AgentKey: "copy", DisplayName: "文案", Status: "active", Roles: []string{"media", "copy"}, PositionID: "pos-copy"},
		{AgentKey: biz.DeptLeadAgentKeyPrefix + "media__", DisplayName: "媒体主管", Status: "active", AgentVariant: "dept_lead", PositionID: "dept-media"},
	}
	reader := &stubAgentReader{agents: agents}
	org := &stubOrgReader{nodes: sampleOrgTree()}
	capBuilder := NewAgentCapabilityBuilder(reader, loggateway.NewNoop())
	capBuilder.SetOrganizationReader(org)
	return &agentAllocatorImpl{
		repo:            &fakeAllocatorRepo{},
		agentReader:     reader,
		capBuilder:      capBuilder,
		lg:              loggateway.NewNoop(),
		staffingAdvisor: advisor,
		agentFactory:    factory,
	}
}

func TestAllocate_StaffingAdoptsInPoolAgent(t *testing.T) {
	adv := &pickStaffingAdvisor{key: "copy"}
	impl := staffingAllocator(t, adv, &fakeAllocatorAgentFactory{agentKey: "factory-x"})
	saved, err := impl.Allocate(context.Background(), unmatchedCopyPlan())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Allocations[0].AssignedKey != "copy" || saved.Allocations[0].MatchLayer != "staffing" {
		t.Fatalf("got %+v", saved.Allocations[0])
	}
	if len(adv.got) != 1 {
		t.Fatalf("staffing calls=%d", len(adv.got))
	}
	ask := adv.got[0]
	if ask.SubTaskName != "任务A" {
		t.Fatalf("must pass subtask name only, got %q", ask.SubTaskName)
	}
	if len(ask.CandidateKeys) == 0 || ask.CandidateKeys[0] != "copy" {
		t.Fatalf("candidates=%v", ask.CandidateKeys)
	}
}

func TestAllocate_StaffingTimeoutFailsClosed(t *testing.T) {
	factory := &fakeAllocatorAgentFactory{agentKey: "factory-x"}
	impl := staffingAllocator(t, blockUntilCancelAdvisor{}, factory)
	impl.staffingTimeout = 20 * time.Millisecond
	_, err := impl.Allocate(context.Background(), unmatchedCopyPlan())
	if err == nil {
		t.Fatal("timeout must fail closed without Factory")
	}
	if len(factory.profiles) != 0 {
		t.Fatalf("factory calls=%d want 0", len(factory.profiles))
	}
}

func TestAllocate_StaffingFactoryFlagFailsClosed(t *testing.T) {
	factory := &fakeAllocatorAgentFactory{agentKey: "factory-x"}
	impl := staffingAllocator(t, factoryStaffingAdvisor{}, factory)
	_, err := impl.Allocate(context.Background(), unmatchedCopyPlan())
	if err == nil {
		t.Fatal("UseFactory must fail closed on the hot path")
	}
	if len(factory.profiles) != 0 {
		t.Fatalf("factory calls=%d want 0", len(factory.profiles))
	}
}

func TestAllocate_DoesNotRewritePlanDAG(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "be", DisplayName: "后端", Status: "active", Roles: []string{"backend"}, PositionID: "pos-be"},
		{AgentKey: "copy", DisplayName: "文案", Status: "active", Roles: []string{"copy"}, PositionID: "pos-copy"},
	}
	reader := &stubAgentReader{agents: agents}
	org := &stubOrgReader{nodes: sampleOrgTree()}
	capBuilder := NewAgentCapabilityBuilder(reader, loggateway.NewNoop())
	capBuilder.SetOrganizationReader(org)
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  capBuilder,
		lg:          loggateway.NewNoop(),
	}
	plan := &biz.TaskPlan{
		ID:       "tp-dag",
		Strategy: biz.StrategyDAG,
		SubTasks: []biz.SubTask{
			{ID: "st_a", Name: "接口", RequiredCapabilities: []string{"backend"}, DomainPath: "软件/后端"},
			{ID: "st_b", Name: "文案", RequiredCapabilities: []string{"copy"}, DomainPath: "创作/文案", DependsOn: []string{"st_a"}},
		},
	}
	before := append([]biz.SubTask(nil), plan.SubTasks...)
	saved, err := impl.Allocate(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SubTasks) != len(before) || plan.SubTasks[1].DependsOn[0] != "st_a" || plan.SubTasks[0].ID != "st_a" {
		t.Fatalf("Allocate must not rewrite Plan DAG: %+v", plan.SubTasks)
	}
	if len(saved.Allocations) != 2 {
		t.Fatalf("allocs=%d", len(saved.Allocations))
	}
}

func TestParseStaffingReply(t *testing.T) {
	got := parseStaffingReply(`{"agent_key":"copy","factory":false}`, []string{"copy", "be"})
	if len(got.AgentKeys) != 1 || got.AgentKeys[0] != "copy" || got.UseFactory {
		t.Fatalf("%+v", got)
	}
	got = parseStaffingReply("```json\n{\"factory\":true}\n```", []string{"copy"})
	if !got.UseFactory {
		t.Fatalf("factory fence: %+v", got)
	}
	got = parseStaffingReply(`{"agent_key":"ghost"}`, []string{"copy"})
	if !got.UseFactory {
		t.Fatalf("unknown key must factory: %+v", got)
	}
}
