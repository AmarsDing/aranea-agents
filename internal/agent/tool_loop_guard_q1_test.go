package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Q1 行为模式闸（2026-08-27 二轮，S02「合法失控」根修）单测：
// 装载闸（重复装载/配额）、wall-time 软/硬闸、plan-execute 漂移观测、
// PolicyResolver 每调用覆盖。

// stubGateDecisionCollector 收集决策记录供断言（漂移/软闸 record-only 观测
// 与硬闸 blocked 记录的验证口）。
type stubGateDecisionCollector struct {
	mu      sync.Mutex
	records []decision.Record
}

func (s *stubGateDecisionCollector) Emit(_ context.Context, rec decision.Record) {
	s.mu.Lock()
	s.records = append(s.records, rec)
	s.mu.Unlock()
}

func (s *stubGateDecisionCollector) count(outcome string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.records {
		if r.Outcome == outcome {
			n++
		}
	}
	return n
}

// toolLoadOK 构造 tool_load 成功结果（与 deferred.toolLoadOutput 序列化形态对齐）。
func toolLoadOK(name string) any {
	return map[string]any{"success": true, "tool_name": name, "message": "loaded"}
}

// backdateGuardEntry 把 ctx 对应守卫条目的 firstSeen 回拨 d——wall-time 闸
// 测试的时间旅行口（避免真实等待软/硬闸秒数）。
func backdateGuardEntry(t *testing.T, g *toolLoopGuard, ctx context.Context, d time.Duration) {
	t.Helper()
	key := loopGuardInvocationKey(ctx)
	if key == "" {
		t.Fatal("invocation key must not be empty")
	}
	g.mu.Lock()
	e := g.entryLocked(key, time.Now())
	e.firstSeen = e.firstSeen.Add(-d)
	g.mu.Unlock()
}

// runWallModelHook 执行一次 BeforeModel hook，返回请求消息（供 wall cue 断言）。
func runWallModelHook(t *testing.T, g *toolLoopGuard, ctx context.Context) []trpcmodel.Message {
	t.Helper()
	hook := g.modelHook()
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: []trpcmodel.Message{
		trpcmodel.NewSystemMessage("sys"),
		trpcmodel.NewUserMessage("hi"),
	}}}
	if _, err := hook.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("model hook error: %v", err)
	}
	return args.Request.Messages
}

func countMarkerMsgs(msgs []trpcmodel.Message, marker string) int {
	n := 0
	for _, m := range msgs {
		if strings.HasPrefix(m.Content, marker) {
			n++
		}
	}
	return n
}

// 装载闸-重复装载：目标已激活的 tool_load 第二次起拦截；不同目标不受影响。
func TestLoopGuardToolLoadRepeatBlockedSecondTime(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-q1-repeat")

	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"exec_command"}`, toolLoadOK("exec_command"), nil); err != nil {
		t.Fatalf("first load should pass: %v", err)
	}
	err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"exec_command"}`, toolLoadOK("exec_command"), nil)
	if err == nil {
		t.Fatal("second load of the same tool must be blocked")
	}
	if !strings.HasPrefix(err.Error(), loopGuardMarker) || !strings.Contains(err.Error(), "exec_command") {
		t.Fatalf("repeat block message should name the target tool, got %q", err.Error())
	}
	if _, ok := trpcagent.AsStopError(err); ok {
		t.Fatal("repeat block must be a plain error before saturation, not StopError")
	}
	// 别名变体重复装载同样拦截（请求名与规范名双记录）。
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"shell_exec"}`, toolLoadOK("exec_command"), nil); err != nil {
		t.Fatalf("alias first request should pass (requested name not yet seen): %v", err)
	}
	// 异质装载不受影响。
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"save_file"}`, toolLoadOK("save_file"), nil); err != nil {
		t.Fatalf("distinct load should pass: %v", err)
	}
}

// 装载闸-配额：默认配额（8）内异质装载放行，第 9 个异质目标拦截。
func TestLoopGuardToolLoadQuotaBlocksNinthDistinct(t *testing.T) {
	g := newToolLoopGuard(nil)
	g.setBootstrapRatio(-1) // 本用例专验次数配额，关闭占比闸
	ctx := newTestInvocationContext("inv-q1-quota")

	for i := 1; i <= loopGuardToolLoadMaxDefault; i++ {
		name := "tool_distinct_" + string(rune('a'+i-1))
		if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, toolLoadOK(name), nil); err != nil {
			t.Fatalf("distinct load %d/%d should pass: %v", i, loopGuardToolLoadMaxDefault, err)
		}
	}
	err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"tool_distinct_over"}`, toolLoadOK("tool_distinct_over"), nil)
	if err == nil {
		t.Fatal("9th distinct load must be blocked by quota")
	}
	if !strings.Contains(err.Error(), "配额上限") {
		t.Fatalf("quota block message should mention the quota, got %q", err.Error())
	}
	// 配额拦截后，已激活工具的重复装载仍走重复拦截（语义更具体），
	// 普通工具调用不受装载闸影响。
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"tool_distinct_a"}`, toolLoadOK("tool_distinct_a"), nil); err == nil ||
		!strings.Contains(err.Error(), "激活状态") {
		t.Fatalf("repeat load should still hit repeat branch, got %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, "eth1 DOWN", nil); err != nil {
		t.Fatalf("non-tool_load call must not be affected by load quota: %v", err)
	}
}

// 装载闸-失败不计：success:false 的装载不记 loadedTools、不占配额，可重试。
// （注：同参同结果的「失败结果」连续重发仍归旧「签名+结果一致」守卫治理，
// 本用例以交错命名规避，专验装载闸自身语义。）
func TestLoopGuardToolLoadFailureNotCounted(t *testing.T) {
	g := newToolLoopGuard(nil)
	g.setBootstrapRatio(-1)
	ctx := newTestInvocationContext("inv-q1-failure")
	notFound := func(name string) any {
		return map[string]any{"success": false, "tool_name": name, "error": "not found"}
	}

	// 同名失败重试不被装载闸拦截（未激活即无「重复装载」可言）。
	for _, name := range []string{"ghost_a", "ghost_b", "ghost_a"} {
		if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, notFound(name), nil); err != nil {
			t.Fatalf("failed load of %s must not be blocked (nothing recorded): %v", name, err)
		}
	}
	// 失败后成功装载 → 记 1 次；再次装载 → 重复拦截。
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"ghost_a"}`, toolLoadOK("ghost_a"), nil); err != nil {
		t.Fatalf("first successful load should pass: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"ghost_a"}`, toolLoadOK("ghost_a"), nil); err == nil {
		t.Fatal("load after success must be blocked as repeat")
	}
	// 三次失败未占配额：再装载 7 个异质工具（连同 ghost_a 共 8=配额）全部放行。
	for i := 1; i < loopGuardToolLoadMaxDefault; i++ {
		name := "tool_fill_" + string(rune('a'+i-1))
		if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, toolLoadOK(name), nil); err != nil {
			t.Fatalf("load %d after failures should pass (failures not counted): %v", i, err)
		}
	}
}

// wall-time 硬闸：节点存续超硬闸 → 任何工具调用（含 todo_declare_blocker）
// 一律 StopError 强终止，并写 outcome=blocked 决策记录。
func TestLoopGuardWallHardGateStopsNode(t *testing.T) {
	g := newToolLoopGuard(nil)
	dc := &stubGateDecisionCollector{}
	g.setDecisionCollector(dc)
	ctx := newTestInvocationContext("inv-q1-wallhard")
	backdateGuardEntry(t, g, ctx, time.Duration(loopGuardWallHardSecDefault+60)*time.Second)

	err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, "x", nil)
	if err == nil {
		t.Fatal("call beyond hard gate must be stopped")
	}
	if _, ok := trpcagent.AsStopError(err); !ok {
		t.Fatalf("hard gate must return StopError, got %v", err)
	}
	// 硬闸无豁免工具（区别于空转封锁的 declare_blocker 投降通道）。
	if err := runLoopGuardTurn(t, g, ctx, todoDeclareBlockerToolName, `{"reason":"x"}`, "ok", nil); err == nil {
		t.Fatal("hard gate must not exempt todo_declare_blocker")
	}
	if got := dc.count("blocked"); got != 2 {
		t.Fatalf("hard gate should emit one blocked record per stopped call, got %d", got)
	}
}

func TestLoopGuardWallHardGateExcludesHITLWait(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := WithConfirmWaitAcc(newTestInvocationContext("inv-q1-wall-hitl"))
	backdateGuardEntry(t, g, ctx, time.Duration(loopGuardWallHardSecDefault+60)*time.Second)
	AddConfirmWaitMS(ctx, (loopGuardWallHardSecDefault+60)*1000)

	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1"}`, "x", nil); err != nil {
		t.Fatalf("HITL wait must not count toward wall-time hard gate, got %v", err)
	}
}

// wall-time 软闸：超软闸后 BeforeModel 每轮注入收尾 cue（同名去重仅一条），
// 首次越线写一条 outcome=tripped 决策记录（once-per-entry）。
func TestLoopGuardWallSoftCueInjectedOnce(t *testing.T) {
	g := newToolLoopGuard(nil)
	dc := &stubGateDecisionCollector{}
	g.setDecisionCollector(dc)
	ctx := newTestInvocationContext("inv-q1-wallsoft")
	backdateGuardEntry(t, g, ctx, time.Duration(loopGuardWallSoftSecDefault+30)*time.Second)

	for round := 1; round <= 3; round++ {
		msgs := runWallModelHook(t, g, ctx)
		if got := countMarkerMsgs(msgs, wallTimeCueMarker); got != 1 {
			t.Fatalf("round %d: wall cue must appear exactly once, got %d", round, got)
		}
	}
	if got := dc.count("tripped"); got != 1 {
		t.Fatalf("wall soft record must be once-per-entry, got %d", got)
	}
	// 未超软闸的节点不注 cue。
	g2 := newToolLoopGuard(nil)
	ctx2 := newTestInvocationContext("inv-q1-wallsoft-fresh")
	if got := countMarkerMsgs(runWallModelHook(t, g2, ctx2), wallTimeCueMarker); got != 0 {
		t.Fatalf("fresh node must not get wall cue, got %d", got)
	}
}

// plan-execute 漂移拦截：plan_and_execute 成功后持续装载达阈值，下一次 tool_load 被拦。
func TestLoopGuardPlanDriftIntercepts(t *testing.T) {
	g := newToolLoopGuard(nil)
	dc := &stubGateDecisionCollector{}
	g.setDecisionCollector(dc)
	ctx := newTestInvocationContext("inv-q1-drift")

	if err := runLoopGuardTurn(t, g, ctx, "plan_and_execute", `{"task_prompt":"t"}`, map[string]any{"plan_id": "tp_1"}, nil); err != nil {
		t.Fatalf("plan call: %v", err)
	}
	for i, name := range []string{"tool_p1", "tool_p2", "tool_p3"} {
		if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, toolLoadOK(name), nil); err != nil {
			t.Fatalf("post-plan load %d should pass: %v", i, err)
		}
	}
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"tool_p4"}`, toolLoadOK("tool_p4"), nil); err == nil {
		t.Fatal("4th post-plan tool_load must be blocked as plan-execute drift")
	}
	g2 := newToolLoopGuard(nil)
	ctx2 := newTestInvocationContext("inv-q1-drift-pre")
	if err := runLoopGuardTurn(t, g2, ctx2, "tool_load", `{"tool_name":"tool_pre"}`, toolLoadOK("tool_pre"), nil); err != nil {
		t.Fatalf("pre-plan load: %v", err)
	}
	if err := runLoopGuardTurn(t, g2, ctx2, "plan_and_execute", `{"task_prompt":"t"}`, map[string]any{"plan_id": "tp_2"}, nil); err != nil {
		t.Fatalf("plan call: %v", err)
	}
	for i, name := range []string{"tool_q1", "tool_q2"} {
		if err := runLoopGuardTurn(t, g2, ctx2, "tool_load", `{"tool_name":"`+name+`"}`, toolLoadOK(name), nil); err != nil {
			t.Fatalf("post-plan load %d: %v", i, err)
		}
	}
}

// PolicyResolver 每调用覆盖：DB 列值（经 resolver）实时改变装载配额，
// 守卫零重建生效；resolver miss 回退构建期快照。
func TestLoopGuardToolLoadResolverOverride(t *testing.T) {
	resetPolicyResolverForTest(t)
	InitPolicyResolver(stubSettingsRepo{all: map[string]biz.AgentRuntimeSettings{
		"agent-q1": {AgentID: "agent-q1", LoopGuardToolLoadMax: 2},
	}}, loggateway.NewNoop())

	g := newToolLoopGuard(nil)
	g.setGateThresholds("agent-q1", 0, 0, 0)
	ctx := newTestInvocationContext("inv-q1-resolver")

	for _, name := range []string{"tool_r1", "tool_r2"} {
		if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, toolLoadOK(name), nil); err != nil {
			t.Fatalf("load within resolver quota 2 should pass: %v", err)
		}
	}
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"tool_r3"}`, toolLoadOK("tool_r3"), nil); err == nil {
		t.Fatal("3rd distinct load must be blocked under resolver quota 2")
	}
	// 增量更新即刻生效（service 层「仅策略字段变化」路径语义）。
	SetLoopGuardGateThresholds("agent-q1", 4, 0, 0)
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"tool_r3"}`, toolLoadOK("tool_r3"), nil); err != nil {
		t.Fatalf("after Set(4), 3rd distinct load should pass: %v", err)
	}

	// resolver 未登记的 agent → 回退构建期快照（build=1 → 第 2 个异质装载拦截）。
	g2 := newToolLoopGuard(nil)
	g2.setGateThresholds("ghost-agent", 1, 0, 0)
	ctx2 := newTestInvocationContext("inv-q1-resolver-fallback")
	if err := runLoopGuardTurn(t, g2, ctx2, "tool_load", `{"tool_name":"tool_f1"}`, toolLoadOK("tool_f1"), nil); err != nil {
		t.Fatalf("first load under build snapshot quota 1 should pass: %v", err)
	}
	if err := runLoopGuardTurn(t, g2, ctx2, "tool_load", `{"tool_name":"tool_f2"}`, toolLoadOK("tool_f2"), nil); err == nil {
		t.Fatal("2nd distinct load must be blocked under build snapshot quota 1")
	}
}

// 并行窗口与装载闸正交：同参并行 tool_load 仍走并行拦截（不计 blockedCount）。
func TestLoopGuardToolLoadParallelWindowUnchanged(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-q1-parallel")
	before := g.beforeHook()
	loadArgs := []byte(`{"tool_name":"exec_command"}`)

	if _, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "tool_load", Arguments: loadArgs}); err != nil {
		t.Fatalf("first call should pass: %v", err)
	}
	// AfterTool 未记账前的同参第二次：并行拦截（不经过装载闸）。
	err := func() error {
		_, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "tool_load", Arguments: loadArgs})
		return err
	}()
	if err == nil || !strings.Contains(err.Error(), "并行") {
		t.Fatalf("parallel duplicate must hit parallel branch, got %v", err)
	}
}

// C1 占比闸：连续 6 次异质 tool_load（窗口满员且 100% 自举）第 6 次拦截。
func TestLoopGuardBootstrapRatioBlocksSixthConsecutiveLoad(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-c1-ratio")
	for i := 1; i <= 5; i++ {
		name := "tool_ratio_" + string(rune('a'+i-1))
		if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, toolLoadOK(name), nil); err != nil {
			t.Fatalf("load %d/5 should pass before ratio window: %v", i, err)
		}
	}
	err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"tool_ratio_over"}`, toolLoadOK("tool_ratio_over"), nil)
	if err == nil {
		t.Fatal("6th consecutive bootstrap must be blocked by ratio gate")
	}
	if !strings.Contains(err.Error(), "50%") && !strings.Contains(err.Error(), "tool_load/tool_search") {
		t.Fatalf("ratio block message should mention bootstrap share, got %q", err.Error())
	}
	// 非自举工具不受占比闸影响。
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"cmd":"show"}`, "ok", nil); err != nil {
		t.Fatalf("non-bootstrap call must pass under ratio gate: %v", err)
	}
}

// 同批并行：tool_load(X) 尚未 AfterTool 时直接 call X → 拦截。
func TestLoopGuardLoadThenCallSameBatchBlocked(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-c1-same-batch")
	before := g.beforeHook()
	if _, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "tool_load", Arguments: []byte(`{"tool_name":"exec_command"}`),
	}); err != nil {
		t.Fatalf("tool_load should pass: %v", err)
	}
	err := func() error {
		_, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
			ToolName: "exec_command", Arguments: []byte(`{"cmd":"ls"}`),
		})
		return err
	}()
	if err == nil || !strings.Contains(err.Error(), "tool_load") {
		t.Fatalf("same-batch call of just-loaded tool must be blocked, got %v", err)
	}
}

// 顺序同一步：tool_load AfterTool 后立刻 call，在下一 BeforeModel 之前拦截。
func TestLoopGuardLoadThenCallSameStepBlockedUntilNextModel(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-c1-same-step")
	if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"save_file"}`, toolLoadOK("save_file"), nil); err != nil {
		t.Fatalf("tool_load should pass: %v", err)
	}
	err := runLoopGuardTurn(t, g, ctx, "save_file", `{"path":"a.txt"}`, "ok", nil)
	if err == nil || !(strings.Contains(err.Error(), "下一轮") || strings.Contains(err.Error(), "tool_load")) {
		t.Fatalf("same-step call after load must be blocked, got %v", err)
	}
	_ = runWallModelHook(t, g, ctx)
	if err := runLoopGuardTurn(t, g, ctx, "save_file", `{"path":"a.txt"}`, "ok", nil); err != nil {
		t.Fatalf("next model step must allow the loaded tool: %v", err)
	}
}

func runFatModelHook(t *testing.T, g *toolLoopGuard, ctx context.Context, body string) []trpcmodel.Message {
	t.Helper()
	hook := g.modelHook()
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: []trpcmodel.Message{
		trpcmodel.NewSystemMessage(body),
		trpcmodel.NewUserMessage("hi"),
	}}}
	if _, err := hook.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("model hook error: %v", err)
	}
	return args.Request.Messages
}

// 轮数×单轮积：软闸注入收尾 cue，硬闸封锁新工具（todo_declare_blocker 豁免）。
func TestLoopGuardRoundProductSoftCueAndHardBlock(t *testing.T) {
	g := newToolLoopGuard(nil)
	dc := &stubGateDecisionCollector{}
	g.setDecisionCollector(dc)
	g.setRoundProduct(500) // 软=350，硬=500
	ctx := newTestInvocationContext("inv-round-prod")
	fat := strings.Repeat("x", 500) // ≈200 tok @ 2.5 chars/token

	round1 := runFatModelHook(t, g, ctx, fat)
	if findMarker(round1, roundProductCueMarker) {
		t.Fatal("round 1 product must stay under soft")
	}
	if err := runLoopGuardTurn(t, g, ctx, "datetime", `{}`, "ok", nil); err != nil {
		t.Fatalf("under hard must still allow tools: %v", err)
	}

	round2 := runFatModelHook(t, g, ctx, fat)
	if !findMarker(round2, roundProductCueMarker) {
		t.Fatal("round 2 product must inject soft wrap-up cue")
	}
	if got := dc.count("tripped"); got != 1 {
		t.Fatalf("soft gate must emit one tripped record, got %d", got)
	}
	if err := runLoopGuardTurn(t, g, ctx, "datetime", `{}`, "ok", nil); err != nil {
		t.Fatalf("between soft and hard must still allow tools: %v", err)
	}

	_ = runFatModelHook(t, g, ctx, fat)
	if err := runLoopGuardTurn(t, g, ctx, "datetime", `{}`, "ok", nil); err == nil {
		t.Fatal("over hard product must block new tools")
	}
	if err := runLoopGuardTurn(t, g, ctx, todoDeclareBlockerToolName, `{"reason":"budget"}`, "ok", nil); err != nil {
		t.Fatalf("todo_declare_blocker must remain exempt: %v", err)
	}

	gOff := newToolLoopGuard(nil)
	gOff.setRoundProduct(-1)
	ctxOff := newTestInvocationContext("inv-round-prod-off")
	for i := 0; i < 5; i++ {
		_ = runFatModelHook(t, gOff, ctxOff, fat)
	}
	if err := runLoopGuardTurn(t, gOff, ctxOff, "datetime", `{}`, "ok", nil); err != nil {
		t.Fatalf("disabled product gate must not block: %v", err)
	}
}
