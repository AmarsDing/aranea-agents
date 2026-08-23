package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// packLikeCopyAgents mirrors agency-pack rows after seed backfill:
// domain_path filled, mission copied from description.
func packLikeCopyAgents() []biz.Agent {
	return []biz.Agent{
		{
			AgentKey: "aeo_foundations__general", DisplayName: "AEO 基础架构师", Status: "active",
			DomainPath: "创作/文案", PositionKey: "aeo_foundations",
			AgentDescription: "AI 引擎优化基础设施专家——实施 llms.txt、AI 感知 robots.txt",
			MissionStatement: "AI 引擎优化基础设施专家——实施 llms.txt、AI 感知 robots.txt",
		},
		{
			AgentKey: "xiaohongshu_specialist__general", DisplayName: "小红书运营专家", Status: "active",
			DomainPath: "创作/文案", PositionKey: "xiaohongshu_specialist",
			AgentDescription: "生活方式内容、趋势策略与小红书增长专家",
			MissionStatement: "生活方式内容、趋势策略与小红书增长专家",
		},
		{
			AgentKey: "content_creator__general", DisplayName: "内容创作者", Status: "active",
			DomainPath: "创作/文案", PositionKey: "content_creator",
			AgentDescription: "多平台内容策略、编辑日历与文案专家",
			MissionStatement: "多平台内容策略、编辑日历与文案专家",
		},
		{
			AgentKey: "book_co_author__general", DisplayName: "图书联合作者", Status: "active",
			DomainPath: "创作/文学", PositionKey: "book_co_author",
			AgentDescription: "思想领导力书籍、代笔写作与出版策略专家",
			MissionStatement: "思想领导力书籍、代笔写作与出版策略专家",
		},
	}
}

func TestEffectiveness_RosterVsL1_Xiaohongshu(t *testing.T) {
	caps := []biz.AgentCapability{
		{AgentKey: "aeo_foundations__general", DisplayName: "AEO 基础架构师", DomainPath: "创作/文案", PositionKey: "aeo_foundations", Mission: "AI 引擎优化基础设施专家——实施 llms.txt"},
		{AgentKey: "xiaohongshu_specialist__general", DisplayName: "小红书运营专家", DomainPath: "创作/文案", PositionKey: "xiaohongshu_specialist", Mission: "生活方式内容、趋势策略与小红书增长专家"},
		{AgentKey: "content_creator__general", DisplayName: "内容创作者", DomainPath: "创作/文案", PositionKey: "content_creator", Mission: "多平台内容策略、编辑日历与文案专家"},
		{AgentKey: "book_co_author__general", DisplayName: "图书联合作者", DomainPath: "创作/文学", PositionKey: "book_co_author", Mission: "思想领导力书籍、代笔写作与出版策略专家"},
	}
	roster, _, ok := bindRosterSpecialist("创作/文案", "小红书种草文案", caps)
	if !ok || roster.AgentKey != "xiaohongshu_specialist__general" {
		t.Fatalf("roster want xiaohongshu, got %+v ok=%v", roster, ok)
	}

	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	l1, score, n, l1ok := impl.tryMissionMatch(context.Background(), "小红书种草文案", "创作/文案", caps, "review")
	t.Logf("L1 ok=%v key=%s score=%.4f cand=%d; roster=%s", l1ok, l1.AgentKey, score, n, roster.AgentKey)
	if l1ok && l1.AgentKey != roster.AgentKey {
		t.Errorf("EFFECT gap: L1=%s but roster=%s — Allocate will take L1 and skip roster scoring", l1.AgentKey, roster.AgentKey)
	}
}

func TestEffectiveness_AllocateLayer_PackLikeXiaohongshu(t *testing.T) {
	reader := &stubAgentReader{agents: packLikeCopyAgents()}
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:          loggateway.NewNoop(),
	}
	saved, err := impl.Allocate(context.Background(), &biz.TaskPlan{
		ID:       "tp-eff-xhs",
		Strategy: biz.StrategyDirect,
		SubTasks: []biz.SubTask{{ID: "st1", Name: "小红书种草文案", DomainPath: "创作/文案"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := saved.Allocations[0]
	t.Logf("Allocate key=%s layer=%s reason=%s", got.AssignedKey, got.MatchLayer, got.MatchReason)
	if got.AssignedKey != "xiaohongshu_specialist__general" || got.MatchLayer != "roster" {
		t.Errorf("EFFECT: want xiaohongshu layer=roster, got %s layer=%s", got.AssignedKey, got.MatchLayer)
	}
}

func TestEffectiveness_AllocateLayer_WriteCopyPicksSpecialistNotAuthor(t *testing.T) {
	reader := &stubAgentReader{agents: packLikeCopyAgents()}
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:          loggateway.NewNoop(),
	}
	saved, err := impl.Allocate(context.Background(), &biz.TaskPlan{
		ID:       "tp-eff-copy",
		Strategy: biz.StrategyDirect,
		SubTasks: []biz.SubTask{{ID: "st1", Name: "写一篇种草文案", DomainPath: "创作/文案"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := saved.Allocations[0]
	t.Logf("Allocate key=%s layer=%s reason=%s", got.AssignedKey, got.MatchLayer, got.MatchReason)
	if got.AssignedKey == "book_co_author__general" {
		t.Errorf("EFFECT: 创作/文案 bound literature author")
	}
}

func TestEffectiveness_OrgPruneAndInfer(t *testing.T) {
	if biz.InferDomainPath("xiaohongshu_specialist", "media_operations", "小红书运营专家") != "创作/文案" {
		t.Fatal("infer xhs")
	}
	if biz.InferDomainPath("alert_handler", "alert_response", "告警处理专家") != "运维/告警" {
		t.Fatal("infer alert")
	}
	caps := []biz.AgentCapability{
		{AgentKey: "xhs", DepartmentID: "d-media", DepartmentKey: "media_operations", DepartmentName: "媒体运营部", DomainPath: "创作/文案"},
		{AgentKey: "be", DepartmentID: "d-be", DepartmentKey: "backend_dev", DepartmentName: "后端开发部", DomainPath: "软件/后端"},
		{AgentKey: "alert", DepartmentID: "d-alert", DepartmentKey: "alert_response", DepartmentName: "告警响应部", DomainPath: "运维/告警"},
	}
	p := OrgPruner{}
	if got := p.Prune("创作/文案", caps); got.FallbackAll || got.DepartmentName != "媒体运营部" {
		t.Fatalf("prune copy: %+v", got)
	}
	if got := p.Prune("运维/告警", caps); got.FallbackAll || got.DepartmentName != "告警响应部" {
		t.Fatalf("prune alert: %+v", got)
	}
	if got := p.Prune("软件/后端", caps); got.FallbackAll || got.DepartmentName != "后端开发部" {
		t.Fatalf("prune be: %+v", got)
	}
}
