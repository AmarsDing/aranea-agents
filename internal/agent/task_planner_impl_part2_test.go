package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
	"testing"
)

// TestAppendClosedLoopPostmortem_Append 验证闭环类（事故/告警/恢复）任务在
// LLM 分解未产出复盘节点时，由引擎确定性追加一个依赖全部叶子节点的复盘
// subtask，且追加后 DAG 仍合法（TS9-GAP-1：TS-9 实跑中复盘靠人工补位）。
func TestAppendClosedLoopPostmortem_Append(t *testing.T) {
	msg := "电商平台订单服务 P2 生产事故：502 错误率 23%，请完成告警分诊、根因定位、修复方案与恢复执行验证"
	subTasks := ts9LikeSubTasks()

	updated, pm, appended := appendClosedLoopPostmortem(msg, subTasks)
	if !appended {
		t.Fatal("closed-loop incident message must append a postmortem subtask")
	}
	if len(updated) != len(subTasks)+1 {
		t.Fatalf("updated len=%d want %d", len(updated), len(subTasks)+1)
	}
	if !strings.Contains(pm.Name, "复盘") {
		t.Errorf("postmortem subtask name=%q should contain 复盘", pm.Name)
	}

	if len(pm.DependsOn) != 1 || pm.DependsOn[0] != "st_4" {
		t.Errorf("postmortem DependsOn=%v want [st_4]（全部叶子节点）", pm.DependsOn)
	}

	if pm.Description == "" {
		t.Error("postmortem description must be non-empty and self-contained")
	}
	if len(pm.Deliverables) == 0 {
		t.Error("postmortem must declare a deliverable contract for the report")
	}
	for _, c := range pm.Deliverables {
		if c.Name == "summary" || c.Name == "cognition" {
			t.Errorf("deliverable contract name %q uses reserved key", c.Name)
		}
	}

	if err := validateSubTaskDAG(updated); err != nil {
		t.Errorf("DAG invalid after postmortem append: %v", err)
	}

	if len(subTasks) != 4 {
		t.Error("appendClosedLoopPostmortem must not mutate the input slice")
	}
}

// TestAppendClosedLoopPostmortem_Skip 验证非闭环任务、节点过少、已含复盘
// 节点三种情况下不追加。
func TestAppendClosedLoopPostmortem_Skip(t *testing.T) {
	t.Run("non-closed-loop message", func(t *testing.T) {
		_, _, appended := appendClosedLoopPostmortem("帮我写一首关于春天的诗并配一张图", ts9LikeSubTasks())
		if appended {
			t.Error("non-incident message must not append postmortem")
		}
	})

	t.Run("fewer than 2 subtasks", func(t *testing.T) {
		single := []biz.SubTask{{ID: "st_1", Name: "告警分诊", Description: "分诊", Priority: 1}}
		_, _, appended := appendClosedLoopPostmortem("P2 生产事故需要恢复", single)
		if appended {
			t.Error("single subtask plan must not append postmortem")
		}
	})

	t.Run("postmortem already present", func(t *testing.T) {
		with := append(ts9LikeSubTasks(), biz.SubTask{
			ID: "st_5", Name: "事故复盘", Description: "复盘", DependsOn: []string{"st_4"}, Priority: 5,
		})
		_, _, appended := appendClosedLoopPostmortem("P2 生产事故需要恢复并复盘", with)
		if appended {
			t.Error("must not duplicate postmortem when LLM already produced one")
		}
	})

	t.Run("knowledge question with 故障诊断思路", func(t *testing.T) {
		msg := "请同时、并行完成：用三句话说明网络巡检指标；用三句话说明日志分析步骤；用三句话说明故障诊断的基本思路。"
		_, _, appended := appendClosedLoopPostmortem(msg, ts9LikeSubTasks())
		if appended {
			t.Error("methodology question must not append a postmortem team")
		}
	})

	t.Run("ops how-to document is not an incident", func(t *testing.T) {
		msg := "组建三人运维专家团队协作完成一份简短的「机房巡检与故障处置」说明文档"
		_, _, appended := appendClosedLoopPostmortem(msg, ts9LikeSubTasks())
		if appended {
			t.Error("inspection how-to document must not append a postmortem team")
		}
	})
}

func TestLooksLikeClosedLoopIncident(t *testing.T) {
	if !looksLikeClosedLoopIncident("交换机宕机，请按告警处置并复盘") {
		t.Fatal("real outage must look like a closed-loop incident")
	}
	if !looksLikeClosedLoopIncident("电商平台订单服务 P2 生产事故：502 错误率 23%") {
		t.Fatal("production incident must match")
	}
	if looksLikeClosedLoopIncident("用三句话说明故障诊断的基本思路") {
		t.Fatal("故障诊断思路 is a knowledge question")
	}
	if looksLikeClosedLoopIncident("写一份机房巡检与故障处置说明") {
		t.Fatal("how-to document is not an incident")
	}
}

func TestMapV1StrategyToV2(t *testing.T) {
	cases := map[biz.OrchestrationStrategy]biz.PlanStrategy{
		biz.StrategyDirect:      biz.PlanStrategySequential,
		biz.StrategySingleAgent: biz.PlanStrategySequential,
		biz.StrategyParallel:    biz.PlanStrategyParallel,
		biz.StrategyDAG:         biz.PlanStrategyDAG,
		biz.StrategyCoordinator: biz.PlanStrategyCoordinator,
	}
	for in, want := range cases {
		if got := mapV1StrategyToV2(in); got != want {
			t.Fatalf("%s → %s want %s", in, got, want)
		}
	}
}

func TestPlan_MemoryHitRecipeReplaySynthesizesSubtask(t *testing.T) {
	msg := "再写一篇小红书种草文案"
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	cache.RecordCompletionWithAgents(context.Background(), biz.ExtractTaskPattern(msg),
		biz.TopologyHybrid, 0.91, 1, 0, []string{"copywriter", "editor"})
	if e, ok := cache.Get(biz.ExtractTaskPattern(msg)); ok {
		e.DomainPath = "创作/内容运营"
	}
	repo := &stubTaskPlanRepo{}
	impl := &taskPlannerImpl{
		repo:      repo,
		orchCache: cache,
		lg:        loggateway.NewNoop(),
	}
	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		UserMessage:     msg,
		SpiritSessionID: "sp-replay",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.MemoryHit == nil || len(plan.MemoryHit.AgentKeysUsed) != 2 {
		t.Fatalf("memory hit keys: %+v", plan.MemoryHit)
	}
	if plan.Strategy != biz.StrategyDAG {
		t.Fatalf("hybrid cache → dag, got %s", plan.Strategy)
	}
	if len(plan.SubTasks) != 1 || plan.SubTasks[0].ID != "st_recipe_replay" {
		t.Fatalf("recipe replay must synthesize one subtask, got %+v", plan.SubTasks)
	}
	if plan.SubTasks[0].DomainPath != "创作/内容运营" {
		t.Fatalf("domain_path %q", plan.SubTasks[0].DomainPath)
	}
	if !strings.Contains(plan.StrategyReason, "配方回放") {
		t.Fatalf("strategy reason %q", plan.StrategyReason)
	}
}

func TestPlan_MemoryHitReplaysSpecialtySlots(t *testing.T) {
	msg := "再做一套种草物料"
	pat := biz.ExtractTaskPattern(msg)
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	cache.RecordCompletionWithAgents(context.Background(), pat,
		biz.TopologyParallel, 0.92, 2, 0, []string{"spec-copy", "spec-des"})
	cache.SetEntrySpecialties(context.Background(), pat, []string{"创作/文案", "设计/视觉"})
	impl := &taskPlannerImpl{
		repo:      &stubTaskPlanRepo{},
		orchCache: cache,
		lg:        loggateway.NewNoop(),
	}
	plan, err := impl.Plan(context.Background(), biz.PlanInput{UserMessage: msg})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SubTasks) != 2 {
		t.Fatalf("slots=%d %+v", len(plan.SubTasks), plan.SubTasks)
	}
	if plan.SubTasks[0].DomainPath != "创作/文案" || plan.SubTasks[1].ID != "st_recipe_2" {
		t.Fatalf("%+v", plan.SubTasks)
	}
}

// ---------------------------------------------------------------------------
// D7-F2 planner 侧一次性降级：受剧本约束任务 + 无 provenance 旧缓存 → keys 弃用，
// 拓扑建议不受影响（fail-closed 只作用于 AgentKeys 复用，不删数据）。
// 夹具：授权剧本显式留空 stages——named 路径命中但 steps 为空而跳过（保持
// planFromNamedPlaybook 短路不触发），指纹仍非空，流程走到 memory 门。
// ---------------------------------------------------------------------------

func TestPlan_FingerprintGateDropsKeysButKeepsTopology(t *testing.T) {
	msg := "请按内容提升流程再写一篇种草文案"
	pat := biz.ExtractTaskPattern(msg)
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	cache.RecordCompletionWithAgents(context.Background(), pat,
		biz.TopologyParallel, 0.92, 2, 0, []string{"spec-copy", "spec-des"})
	org := &stubOrgReader{nodes: map[string]biz.OrganizationNode{
		"co-1": {
			ID: "co-1", Key: "acme", Level: "company",
			MetadataJSON: `{"playbooks": [{"id": "content_lift", "name": "内容提升", "authorized_by": "__company_lead_acme__"}]}`,
		},
	}}
	impl := &taskPlannerImpl{
		repo:      &stubTaskPlanRepo{},
		orchCache: cache,
		lg:        loggateway.NewNoop(),
	}
	AttachPlannerOrganizationReader(impl, org)

	plan, err := impl.Plan(context.Background(), biz.PlanInput{UserMessage: msg, SpiritSessionID: "sp-fp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.MemoryHit == nil {
		t.Fatal("受约束任务命中旧缓存仍应成立（只降级 keys，不作废拓扑记忆）")
	}
	if len(plan.MemoryHit.AgentKeysUsed) != 0 {
		t.Fatalf("无 provenance 旧缓存面对受约束任务必须弃用 keys，got %v", plan.MemoryHit.AgentKeysUsed)
	}
	if plan.Strategy != biz.StrategyParallel {
		t.Fatalf("拓扑建议必须保留（parallel → StrategyParallel），got %s", plan.Strategy)
	}
	if len(plan.SubTasks) != 0 {
		t.Fatalf("keys 被降级后不得合成配方回放子任务，got %+v", plan.SubTasks)
	}
}

func TestPlan_NoPlaybookConstraintKeepsLegacyKeys(t *testing.T) {
	// 对照组：无组织/无剧本约束（wantFp==""）→ 旧缓存 keys 继续复用（兼容语义）。
	msg := "请再写一篇种草文案"
	pat := biz.ExtractTaskPattern(msg)
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	cache.RecordCompletionWithAgents(context.Background(), pat,
		biz.TopologyParallel, 0.92, 2, 0, []string{"spec-copy", "spec-des"})
	impl := &taskPlannerImpl{
		repo:      &stubTaskPlanRepo{},
		orchCache: cache,
		lg:        loggateway.NewNoop(),
	}
	plan, err := impl.Plan(context.Background(), biz.PlanInput{UserMessage: msg, SpiritSessionID: "sp-fp-2"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.MemoryHit == nil || len(plan.MemoryHit.AgentKeysUsed) != 2 {
		t.Fatalf("无约束任务必须继续复用旧缓存 keys，hit=%+v", plan.MemoryHit)
	}
	if len(plan.SubTasks) == 0 {
		t.Fatal("keys 保留时必须合成配方回放子任务")
	}
}

// ---------------------------------------------------------------------------
// D7-F6 planner 记忆读按 workspace 隔离：ws-a 写入的配方对 ws-b 调用方 miss。
// ---------------------------------------------------------------------------

func TestQueryMemory_CrossTenantMisses(t *testing.T) {
	msg := "再写一篇小红书种草文案"
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	ctxA := workspace.WithContext(context.Background(), "ws-a")
	cache.RecordCompletionWithAgents(ctxA, biz.ExtractTaskPattern(msg),
		biz.TopologyParallel, 0.92, 1, 0, []string{"copywriter"})
	impl := &taskPlannerImpl{
		repo:      &stubTaskPlanRepo{},
		orchCache: cache,
		lg:        loggateway.NewNoop(),
	}

	ctxB := workspace.WithContext(context.Background(), "ws-b")
	if hit := impl.queryMemory(ctxB, biz.PlanInput{UserMessage: msg, SpiritSessionID: "sp-ws-b"}, "trace-ws-1"); hit != nil {
		t.Fatalf("ws-a 配方对 ws-b 必须 miss，got %+v", hit)
	}
	if hit := impl.queryMemory(ctxA, biz.PlanInput{UserMessage: msg, SpiritSessionID: "sp-ws-a"}, "trace-ws-2"); hit == nil {
		t.Fatal("ws-a 配方对 ws-a 必须 hit")
	}
}

func TestPlan_SubIntents_EmitsOrderedSlots(t *testing.T) {
	impl := NewTaskPlanner(&stubTaskPlanRepo{}, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil)
	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "sp-sub",
		UserMessage:     "查金鹏科技最近行情并整理成邮件发给运维",
		IntentArtifact: &biz.IntentArtifact{
			RefinedGoal: "查金鹏科技最近行情",
			IntentKind:  "research",
			SubIntents: []biz.SubIntent{
				{Goal: "查金鹏科技最近行情", IntentKind: "research", ToolHints: []string{"web_research"}},
				{Goal: "整理成邮件发给运维", IntentKind: "task", ToolHints: []string{"email_send"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.DecomposeReason != "composite sub_intents; skip planner LLM" {
		t.Fatalf("DecomposeReason=%q", plan.DecomposeReason)
	}
	if plan.Strategy != biz.StrategyDAG {
		t.Fatalf("Strategy=%s want dag", plan.Strategy)
	}
	if len(plan.SubTasks) != 2 {
		t.Fatalf("subtasks=%d", len(plan.SubTasks))
	}
	if plan.SubTasks[0].DomainPath != "研究/调研" {
		t.Fatalf("slot0 domain=%q", plan.SubTasks[0].DomainPath)
	}
	if len(plan.SubTasks[1].DependsOn) != 1 || plan.SubTasks[1].DependsOn[0] != plan.SubTasks[0].ID {
		t.Fatalf("slot1 depends=%v", plan.SubTasks[1].DependsOn)
	}
}
