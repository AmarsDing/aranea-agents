package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// ─── G1 计划校验门：纯函数 verifyPlanFeasibility ─────────────────────

func verifierCaps() []biz.AgentCapability {
	return []biz.AgentCapability{
		{AgentKey: "agent-be", Roles: []string{"go-backend", "database"}},
		{AgentKey: "agent-fe", Roles: []string{"vue3-frontend", "quasar-ui"}},
	}
}

func TestVerifyPlanFeasibility_AllSatisfiable(t *testing.T) {
	subTasks := []biz.SubTask{
		{ID: "st_1", Name: "后端开发", Description: "实现 API", RequiredCapabilities: []string{"go-backend"}},
		{ID: "st_2", Name: "前端开发", Description: "实现页面", RequiredCapabilities: []string{"VUE3-Frontend"}}, // 大小写不敏感
	}
	if v := verifyPlanFeasibility(subTasks, verifierCaps(), 0); len(v) != 0 {
		t.Fatalf("violations = %+v, want none", v)
	}
}

func TestVerifyPlanFeasibility_CapabilityUnsatisfiable(t *testing.T) {
	subTasks := []biz.SubTask{
		{ID: "st_1", Name: "硬件维修", Description: "上门修服务器", RequiredCapabilities: []string{"hardware-repair"}},
	}
	v := verifyPlanFeasibility(subTasks, verifierCaps(), 0)
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1 (%+v)", len(v), v)
	}
	if v[0].Rule != PlanViolationCapabilityUnsatisfiable {
		t.Fatalf("rule = %q, want %q", v[0].Rule, PlanViolationCapabilityUnsatisfiable)
	}
	if v[0].SubTaskID != "st_1" {
		t.Fatalf("subtask = %q, want st_1", v[0].SubTaskID)
	}
	if !strings.Contains(v[0].Detail, "hardware-repair") {
		t.Fatalf("detail %q must name the missing tag", v[0].Detail)
	}
}

func TestVerifyPlanFeasibility_EmptyDefinition(t *testing.T) {
	subTasks := []biz.SubTask{
		{ID: "st_1", Name: "  ", Description: "有描述"},
		{ID: "st_2", Name: "有名字", Description: ""},
	}
	v := verifyPlanFeasibility(subTasks, verifierCaps(), 0)
	if len(v) != 2 {
		t.Fatalf("violations = %d, want 2 (%+v)", len(v), v)
	}
	for _, item := range v {
		if item.Rule != PlanViolationEmptyDefinition {
			t.Fatalf("rule = %q, want %q", item.Rule, PlanViolationEmptyDefinition)
		}
	}
}

func TestVerifyPlanFeasibility_OversizedPlan(t *testing.T) {
	var subTasks []biz.SubTask
	for i := 0; i < maxVerifiedSubTasks+1; i++ {
		subTasks = append(subTasks, biz.SubTask{ID: "st_x", Name: "n", Description: "d"})
	}
	v := verifyPlanFeasibility(subTasks, verifierCaps(), 0)
	if len(v) != 1 || v[0].Rule != PlanViolationOversizedPlan {
		t.Fatalf("violations = %+v, want single oversized_plan", v)
	}
}

// R4 team_count_mismatch：用户显式请求 N 个团队时子任务数必须恰好为 N。
func TestVerifyPlanFeasibility_TeamCountMismatch(t *testing.T) {
	three := []biz.SubTask{
		{ID: "st_1", Name: "创作A", Description: "写诗 A"},
		{ID: "st_2", Name: "创作B", Description: "写诗 B"},
		{ID: "st_3", Name: "测评", Description: "选优"},
	}
	// 数量一致 → 无违例
	if v := verifyPlanFeasibility(three, verifierCaps(), 3); len(v) != 0 {
		t.Fatalf("matched: violations = %+v, want none", v)
	}
	// 超出 → 计划级违例（SubTaskID 为空）
	four := append(append([]biz.SubTask{}, three...), biz.SubTask{ID: "st_4", Name: "了解现状", Description: "统计"})
	v := verifyPlanFeasibility(four, verifierCaps(), 3)
	if len(v) != 1 || v[0].Rule != PlanViolationTeamCountMismatch {
		t.Fatalf("over: violations = %+v, want single team_count_mismatch", v)
	}
	if v[0].SubTaskID != "" {
		t.Fatalf("team_count_mismatch must be plan-level, got subtask %q", v[0].SubTaskID)
	}
	// 不足 → 同样违例
	if v := verifyPlanFeasibility(three[:2], verifierCaps(), 3); len(v) != 1 || v[0].Rule != PlanViolationTeamCountMismatch {
		t.Fatalf("under: violations = %+v, want single team_count_mismatch", v)
	}
	// teamCount=0（用户未显式请求）→ R4 不适用
	if v := verifyPlanFeasibility(four, verifierCaps(), 0); len(v) != 0 {
		t.Fatalf("teamCount=0: violations = %+v, want none", v)
	}
}

func TestVerifyPlanFeasibility_EdgeCases(t *testing.T) {
	// 无 RequiredCapabilities → 不做能力校验（LLM 未声明时信任分配器兜底）。
	subTasks := []biz.SubTask{{ID: "st_1", Name: "n", Description: "d"}}
	if v := verifyPlanFeasibility(subTasks, verifierCaps(), 0); len(v) != 0 {
		t.Fatalf("no-required-cap: violations = %+v, want none", v)
	}
	// 能力清单为空（全系统无业务 agent）→ R2 不适用，不产生误报。
	subTasks[0].RequiredCapabilities = []string{"go-backend"}
	if v := verifyPlanFeasibility(subTasks, nil, 0); len(v) != 0 {
		t.Fatalf("empty-capability-inventory: violations = %+v, want none", v)
	}
	// 空计划 → 无违例（空结果由 decompose_empty 分支处理，非校验门职责）。
	if v := verifyPlanFeasibility(nil, verifierCaps(), 0); len(v) != 0 {
		t.Fatalf("nil-plan: violations = %+v, want none", v)
	}
}

func TestFormatViolationsForRetry(t *testing.T) {
	violations := []PlanViolation{
		{Rule: PlanViolationCapabilityUnsatisfiable, SubTaskID: "st_1", Detail: "required_capabilities [hardware-repair] 无任何 Agent 具备"},
	}
	feedback := formatViolationsForRetry(violations)
	if !strings.Contains(feedback, "st_1") || !strings.Contains(feedback, "hardware-repair") {
		t.Fatalf("feedback must contain subtask id and missing tag, got: %q", feedback)
	}
}

// ─── G1 计划校验门：planner 集成（修复 / 降级）────────────────────────

// gateTestPlanner 构造带校验门的最小 planner：capBuilder 由 stubAgentReader
// 提供能力清单；repairDecomposeFn 由用例注入以模拟修复尝试结果。
func gateTestPlanner(agents []biz.Agent, repairFn func(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, level biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error)) *taskPlannerImpl {
	reader := &stubAgentReader{agents: agents}
	return &taskPlannerImpl{
		repo:              &stubTaskPlanRepo{},
		lg:                loggateway.NewNoop(),
		capBuilder:        NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		repairDecomposeFn: repairFn,
	}
}

func TestPlanVerifyGate_RepairSuccess(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "agent-be", Status: "active", Roles: []string{"go-backend"}},
	}
	good := []biz.SubTask{
		{ID: "st_1", Name: "后端", Description: "修 API", RequiredCapabilities: []string{"go-backend"}},
	}
	impl := gateTestPlanner(agents, func(_ context.Context, msg string, _ *biz.IntentArtifact, _ int, _ biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		// 修复 prompt 必须携带违例反馈（Self-Refine：失败上下文写回）。
		if !strings.Contains(msg, "hardware-repair") {
			t.Errorf("repair prompt missing violation feedback: %q", msg)
		}
		return good, buildDAGFromSubTasks(good), nil
	})

	bad := []biz.SubTask{
		{ID: "st_1", Name: "硬件", Description: "修服务器", RequiredCapabilities: []string{"hardware-repair"}},
	}
	out := impl.applyPlanVerifyGate(context.Background(), bad, buildDAGFromSubTasks(bad), biz.PlanInput{UserMessage: "orig"}, 0, biz.ComplexityComplex)
	if out.degraded {
		t.Fatalf("degraded = true, want repair success; note=%q", out.note)
	}
	if len(out.subTasks) != 1 || out.subTasks[0].RequiredCapabilities[0] != "go-backend" {
		t.Fatalf("subTasks = %+v, want repaired plan", out.subTasks)
	}
	if out.note == "" {
		t.Fatal("note empty, want repair annotation appended to decomposeReason")
	}
}

func TestPlanVerifyGate_RepairStillInvalid_Degrades(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "agent-be", Status: "active", Roles: []string{"go-backend"}},
	}
	stillBad := []biz.SubTask{
		{ID: "st_1", Name: "硬件", Description: "修服务器", RequiredCapabilities: []string{"hardware-repair"}},
	}
	impl := gateTestPlanner(agents, func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _ biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		return stillBad, buildDAGFromSubTasks(stillBad), nil
	})

	bad := []biz.SubTask{
		{ID: "st_1", Name: "硬件", Description: "修服务器", RequiredCapabilities: []string{"hardware-repair"}},
	}
	out := impl.applyPlanVerifyGate(context.Background(), bad, buildDAGFromSubTasks(bad), biz.PlanInput{UserMessage: "orig"}, 0, biz.ComplexityComplex)
	if !out.degraded {
		t.Fatal("degraded = false, want degrade after bounded repair still invalid")
	}
	if out.subTasks != nil {
		t.Fatalf("subTasks = %+v, want nil on degrade", out.subTasks)
	}
	if !strings.Contains(out.note, "hardware-repair") {
		t.Fatalf("degrade note must carry violation detail, got %q", out.note)
	}
}

func TestPlanVerifyGate_NoViolations_Passthrough(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "agent-be", Status: "active", Roles: []string{"go-backend"}},
	}
	called := false
	impl := gateTestPlanner(agents, func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _ biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		called = true
		return nil, nil, nil
	})
	ok := []biz.SubTask{
		{ID: "st_1", Name: "后端", Description: "修 API", RequiredCapabilities: []string{"go-backend"}},
	}
	out := impl.applyPlanVerifyGate(context.Background(), ok, buildDAGFromSubTasks(ok), biz.PlanInput{UserMessage: "orig"}, 0, biz.ComplexityComplex)
	if out.degraded || called {
		t.Fatalf("passthrough expected: degraded=%v repairCalled=%v", out.degraded, called)
	}
	if len(out.subTasks) != 1 {
		t.Fatalf("subTasks = %+v, want original plan", out.subTasks)
	}
}

// nil capBuilder（旧测试构造路径）→ 校验门整体跳过，行为与旧版一致。
func TestPlanVerifyGate_NilCapBuilder_Skips(t *testing.T) {
	impl := &taskPlannerImpl{lg: loggateway.NewNoop()}
	bad := []biz.SubTask{{ID: "st_1", Name: "x", Description: "y", RequiredCapabilities: []string{"nonexistent"}}}
	out := impl.applyPlanVerifyGate(context.Background(), bad, buildDAGFromSubTasks(bad), biz.PlanInput{}, 0, biz.ComplexityComplex)
	if out.degraded {
		t.Fatal("nil capBuilder must skip the gate (fail-open), not degrade")
	}
}

// ─── R4 团队数量不符：校验门集成 ────────────────────────────────────

func teamCountSubTasks(n int) []biz.SubTask {
	names := []string{"诗歌创作A", "诗歌创作B", "测评选优", "了解现状", "额外任务"}
	out := make([]biz.SubTask, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, biz.SubTask{ID: "st_" + string(rune('1'+i)), Name: names[i], Description: "d"})
	}
	return out
}

// 数量超出 → 有界重分解修复为恰好 N 个 → 通过且不降级。
func TestPlanVerifyGate_TeamCountMismatch_RepairSuccess(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "agent-be", Status: "active", Roles: []string{"go-backend"}},
	}
	repaired := teamCountSubTasks(3)
	impl := gateTestPlanner(agents, func(_ context.Context, msg string, _ *biz.IntentArtifact, teamCount int, _ biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		if teamCount != 3 {
			t.Errorf("repairFn teamCount = %d, want 3", teamCount)
		}
		// 修复 prompt 必须携带数量违例反馈与硬约束。
		if !strings.Contains(msg, "team_count_mismatch") || !strings.Contains(msg, "完全一致") {
			t.Errorf("repair prompt missing team-count feedback: %q", msg)
		}
		return repaired, buildDAGFromSubTasks(repaired), nil
	})

	over := teamCountSubTasks(4)
	out := impl.applyPlanVerifyGate(context.Background(), over, buildDAGFromSubTasks(over), biz.PlanInput{UserMessage: "orig"}, 3, biz.ComplexityComplex)
	if out.degraded {
		t.Fatalf("degraded = true, want repair success; note=%q", out.note)
	}
	if len(out.subTasks) != 3 {
		t.Fatalf("subTasks = %d, want repaired 3", len(out.subTasks))
	}
}

// 修复后数量仍不符 → 不降级 direct，返回修复后产物交数量兜底。
func TestPlanVerifyGate_TeamCountMismatch_StillMismatch_NoDegrade(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "agent-be", Status: "active", Roles: []string{"go-backend"}},
	}
	stillOver := teamCountSubTasks(4)
	impl := gateTestPlanner(agents, func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _ biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		return stillOver, buildDAGFromSubTasks(stillOver), nil
	})

	over := teamCountSubTasks(5)
	out := impl.applyPlanVerifyGate(context.Background(), over, buildDAGFromSubTasks(over), biz.PlanInput{UserMessage: "orig"}, 3, biz.ComplexityComplex)
	if out.degraded {
		t.Fatal("degraded = true, want passthrough to count fallback")
	}
	if len(out.subTasks) != 4 {
		t.Fatalf("subTasks = %d, want repaired 4", len(out.subTasks))
	}
	if !strings.Contains(out.note, "数量仍不符") {
		t.Fatalf("note must annotate count fallback, got %q", out.note)
	}
}

// 修复调用本身失败且原始违例仅数量不符 → 不降级，返回原计划交数量兜底。
func TestPlanVerifyGate_TeamCountMismatch_RepairError_NoDegrade(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "agent-be", Status: "active", Roles: []string{"go-backend"}},
	}
	impl := gateTestPlanner(agents, func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _ biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		return nil, nil, context.DeadlineExceeded
	})

	over := teamCountSubTasks(4)
	out := impl.applyPlanVerifyGate(context.Background(), over, buildDAGFromSubTasks(over), biz.PlanInput{UserMessage: "orig"}, 3, biz.ComplexityComplex)
	if out.degraded {
		t.Fatal("degraded = true, want passthrough to count fallback on repair error")
	}
	if len(out.subTasks) != 4 {
		t.Fatalf("subTasks = %d, want original 4", len(out.subTasks))
	}
}

// 数量违例叠加能力违例且修复未果 → 维持降级 direct（非数量独例不享受兜底）。
func TestPlanVerifyGate_MixedViolations_StillDegrade(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "agent-be", Status: "active", Roles: []string{"go-backend"}},
	}
	stillBad := append(teamCountSubTasks(4), biz.SubTask{ID: "st_9", Name: "硬件", Description: "修服务器", RequiredCapabilities: []string{"hardware-repair"}})
	impl := gateTestPlanner(agents, func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _ biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		return stillBad, buildDAGFromSubTasks(stillBad), nil
	})

	mixed := append(teamCountSubTasks(2), biz.SubTask{ID: "st_8", Name: "硬件", Description: "修服务器", RequiredCapabilities: []string{"hardware-repair"}})
	out := impl.applyPlanVerifyGate(context.Background(), mixed, buildDAGFromSubTasks(mixed), biz.PlanInput{UserMessage: "orig"}, 3, biz.ComplexityComplex)
	if !out.degraded {
		t.Fatal("degraded = false, want degrade for mixed violations")
	}
}

// ─── G5 策略决策证据链 ────────────────────────────────────────────────

func flowLogSteps(bus *stubMonitorBus, stepID string) []contract.MonitorEvent {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	var out []contract.MonitorEvent
	for _, ev := range bus.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			continue
		}
		if step, _ := ev.Metadata["step_id"].(string); step == stepID {
			out = append(out, ev)
		}
	}
	return out
}

func decisionCtx(bus *stubMonitorBus) context.Context {
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: bus}, event.TraceContext{
		TraceID: "tr-g5",
		Domain:  event.TraceDomainChat,
	}, nil)
	return event.WithTraceEmitter(context.Background(), em)
}

// 显式 mode：证据链记录 decision_source=llm_mode 与最终策略。
func TestTaskPlanner_Plan_EmitsDecisionEvidence_ExplicitMode(t *testing.T) {
	repo := &stubTaskPlanRepo{}
	bus := &captureNoticeBus{}
	monBus := &stubMonitorBus{}
	impl := NewTaskPlanner(repo, nil, nil, bus, nil, loggateway.NewNoop(), nil, nil, nil)

	_, err := impl.Plan(decisionCtx(monBus), biz.PlanInput{
		SpiritSessionID: "sp-g5",
		UserMessage:     "并行分析 A 和 B",
		Mode:            "parallel",
	})
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	evs := flowLogSteps(monBus, "spirit.planner.decision")
	if len(evs) != 1 {
		t.Fatalf("spirit.planner.decision events = %d, want 1", len(evs))
	}
	md := evs[0].Metadata
	if md["decision_source"] != "llm_mode" {
		t.Fatalf("decision_source = %v, want llm_mode", md["decision_source"])
	}
	if md["strategy"] != string(biz.StrategyParallel) {
		t.Fatalf("strategy = %v, want parallel", md["strategy"])
	}
	if _, ok := md["complexity_score"]; !ok {
		t.Fatal("complexity_score missing from evidence")
	}
}

// LLM 未传 mode + 用户消息含团队关键词：证据链记录 keyword_fallback。
func TestTaskPlanner_Plan_EmitsDecisionEvidence_KeywordFallback(t *testing.T) {
	repo := &stubTaskPlanRepo{}
	bus := &captureNoticeBus{}
	monBus := &stubMonitorBus{}
	impl := NewTaskPlanner(repo, nil, nil, bus, nil, loggateway.NewNoop(), nil, nil, nil)

	_, err := impl.Plan(decisionCtx(monBus), biz.PlanInput{
		SpiritSessionID: "sp-g5b",
		UserMessage:     "组建团队完成代码重构",
		Mode:            "",
	})
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	evs := flowLogSteps(monBus, "spirit.planner.decision")
	if len(evs) != 1 {
		t.Fatalf("spirit.planner.decision events = %d, want 1", len(evs))
	}
	md := evs[0].Metadata
	if md["decision_source"] != "keyword_fallback" {
		t.Fatalf("decision_source = %v, want keyword_fallback", md["decision_source"])
	}
	if md["mode"] != "dag" {
		t.Fatalf("mode = %v, want dag (upgraded by keyword fallback)", md["mode"])
	}
}

// 无 emitter 的 ctx（后台路径）不得 panic。
func TestTaskPlanner_Plan_DecisionEvidence_NoEmitter(t *testing.T) {
	repo := &stubTaskPlanRepo{}
	impl := NewTaskPlanner(repo, nil, nil, &captureNoticeBus{}, nil, loggateway.NewNoop(), nil, nil, nil)
	if _, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "sp-g5c",
		UserMessage:     "你好",
		Mode:            "direct",
	}); err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
}
