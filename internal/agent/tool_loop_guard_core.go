package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/tools/alias"
	"aranea-agents/pkg/loggateway"
)

type toolLoopGuard struct {
	mu      sync.Mutex
	entries map[string]*loopGuardEntry
	lg      loggateway.Logger
	// decisions 是 M80 系统闸决策双写口（设计 §3.2 row 3 loop_guard_blocked）。
	// 可选：nil 时拦截仅回灌错误消息，不产决策记录。
	decisions decision.Collector
	// Q1 行为模式闸阈值的每调用解析入参：policyAgentID 是 resolver 查询键，
	// build* 是构建期快照（resolver miss 兜底；≤0 归一内置默认）。装配点
	// callback_chain.go 经 setGateThresholds 注入（与 setDecisionCollector 同型，
	// 保持 newToolLoopGuard(lg) 签名不变，存量测试零改动）。
	policyAgentID string
	buildLoadMax  int
	buildWallSoft int
	buildWallHard int
	// buildBootstrapRatio：<0 测试关闭占比闸；0 用默认 0.5；>0 覆盖。
	buildBootstrapRatio float64
	// buildPlanDrift：<0 测试关闭漂移拦截；0 用默认观察阈值并拦截；>0 覆盖阈值。
	buildPlanDrift int
	// buildRoundProduct：<0 关闭轮数积闸；0 用默认 350K/500K；>0 覆盖硬阈（软=70%）。
	buildRoundProduct int
}

func newToolLoopGuard(lg loggateway.Logger) *toolLoopGuard {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &toolLoopGuard{entries: map[string]*loopGuardEntry{}, lg: lg}
}

// setDecisionCollector wires the M80 decision collector (callback_chain.go
// 装配点）。保持 newToolLoopGuard(lg) 签名不变，存量测试零改动。
func (g *toolLoopGuard) setDecisionCollector(c decision.Collector) {
	if g == nil {
		return
	}
	g.decisions = c
}

// setGateThresholds 注入 Q1 行为模式闸的每调用解析入参（callback_chain.go
// 装配点）：agentID 作 resolver 查询键，build* 为构建期快照兜底。阈值本体
// 不固化进守卫——每调用经 loopGuardToolLoadMaxFor / loopGuardWallSecFor 查询
// （resolver 命中用 resolver 值），策略变更零重建生效。
func (g *toolLoopGuard) setGateThresholds(agentID string, loadMax, wallSoftSec, wallHardSec int) {
	if g == nil {
		return
	}
	g.policyAgentID = agentID
	g.buildLoadMax = loadMax
	g.buildWallSoft = wallSoftSec
	g.buildWallHard = wallHardSec
}

// 每调用阈值解析（Q1）：resolver 优先、构建期快照兜底、≤0 归一内置默认。
func (g *toolLoopGuard) loadMaxFor() int {
	return loopGuardToolLoadMaxFor(g.policyAgentID, g.buildLoadMax)
}

func (g *toolLoopGuard) wallSecFor(hard bool) int {
	build := g.buildWallSoft
	if hard {
		build = g.buildWallHard
	}
	return loopGuardWallSecFor(g.policyAgentID, build, hard)
}

// wallElapsedNet is node wall time minus HITL confirmation wait. S05 turns
// spent 300s of 314s waiting on confirm; counting that toward the 240/600s
// gates false-kills paused nodes (campaign lesson: wall-time must exclude wait).
func wallElapsedNet(ctx context.Context, firstSeen, now time.Time) time.Duration {
	elapsed := now.Sub(firstSeen)
	if elapsed < 0 {
		return 0
	}
	wait := time.Duration(ConfirmWaitMS(ctx)) * time.Millisecond
	if wait <= 0 {
		return elapsed
	}
	if wait >= elapsed {
		return 0
	}
	return elapsed - wait
}

func (g *toolLoopGuard) setBootstrapRatio(r float64) {
	if g == nil {
		return
	}
	g.buildBootstrapRatio = r
}

func (g *toolLoopGuard) setPlanDrift(n int) {
	if g == nil {
		return
	}
	g.buildPlanDrift = n
}

func (g *toolLoopGuard) planDriftAt() int {
	if g == nil {
		return loopGuardPlanDriftObserveAt
	}
	if g.buildPlanDrift < 0 {
		return 0
	}
	if g.buildPlanDrift > 0 {
		return g.buildPlanDrift
	}
	return loopGuardPlanDriftObserveAt
}

func (g *toolLoopGuard) setRoundProduct(hard int) {
	if g == nil {
		return
	}
	g.buildRoundProduct = hard
}

func (g *toolLoopGuard) roundProductHard() int {
	if g == nil {
		return loopGuardRoundProductHardDefault
	}
	if g.buildRoundProduct < 0 {
		return 0
	}
	if g.buildRoundProduct > 0 {
		return g.buildRoundProduct
	}
	return loopGuardRoundProductHardDefault
}

func (g *toolLoopGuard) roundProductSoft() int {
	hard := g.roundProductHard()
	if hard <= 0 {
		return 0
	}
	return int(float64(hard) * 0.7)
}

func (g *toolLoopGuard) bootstrapRatioFor() float64 {
	if g == nil {
		return loopGuardBootstrapRatioDefault
	}
	if g.buildBootstrapRatio < 0 {
		return 0
	}
	if g.buildBootstrapRatio > 0 {
		return g.buildBootstrapRatio
	}
	return loopGuardBootstrapRatioDefault
}

func loopGuardIsBootstrapTool(name string) bool {
	return name == toolLoadToolName || name == toolSearchToolName
}

func loopGuardResolveAlias(name string) string {
	name = strings.TrimSpace(name)
	for i := 0; i < 8 && name != ""; i++ {
		next, ok := alias.RuntimeToolNameAliases[name]
		if !ok || next == "" || next == name {
			return name
		}
		name = next
	}
	return name
}

func loopGuardNameKeys(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	seen := map[string]struct{}{name: {}, loopGuardResolveAlias(name): {}}
	for aliasName, target := range alias.RuntimeToolNameAliases {
		if target == name || aliasName == name || loopGuardResolveAlias(aliasName) == loopGuardResolveAlias(name) {
			seen[aliasName] = struct{}{}
			seen[target] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

func loopGuardBootstrapRatioTripped(recentTools []string, next string, ratio float64) bool {
	if ratio <= 0 || !loopGuardIsBootstrapTool(next) {
		return false
	}
	w := append(append([]string(nil), recentTools...), next)
	if len(w) > loopGuardBootstrapWindow {
		w = w[len(w)-loopGuardBootstrapWindow:]
	}
	if len(w) < loopGuardBootstrapMinCalls {
		return false
	}
	n := 0
	for _, t := range w {
		if loopGuardIsBootstrapTool(t) {
			n++
		}
	}
	return float64(n) >= float64(len(w))*ratio
}

func (e *loopGuardEntry) beginInflightLoad(target string) {
	if target == "" {
		return
	}
	if e.inflightLoads == nil {
		e.inflightLoads = map[string]int{}
	}
	for _, k := range loopGuardNameKeys(target) {
		e.inflightLoads[k]++
	}
}

func (e *loopGuardEntry) endInflightLoad(target string) {
	if e.inflightLoads == nil || target == "" {
		return
	}
	for _, k := range loopGuardNameKeys(target) {
		if e.inflightLoads[k] <= 1 {
			delete(e.inflightLoads, k)
			continue
		}
		e.inflightLoads[k]--
	}
}

func (e *loopGuardEntry) markJustLoaded(target string) {
	if target == "" {
		return
	}
	if e.justLoaded == nil {
		e.justLoaded = map[string]bool{}
	}
	for _, k := range loopGuardNameKeys(target) {
		e.justLoaded[k] = true
	}
}

func (e *loopGuardEntry) loadThenCallTarget(toolName string) string {
	if toolName == "" || loopGuardIsBootstrapTool(toolName) {
		return ""
	}
	for _, k := range loopGuardNameKeys(toolName) {
		if e.inflightLoads[k] > 0 || e.justLoaded[k] {
			return k
		}
	}
	return ""
}

// emitGateDecision 把一次拦截/强停双写为 system_guard 决策记录。
// observed/threshold/action 由调用点按分支语义给出。
func (g *toolLoopGuard) emitGateDecision(ctx context.Context, toolName, scenario, reasoning string, observed any, threshold any, action string) {
	g.emitGateEvent(ctx, "blocked", toolName, scenario, reasoning, observed, threshold, action)
}

// emitGateEvent 是 emitGateDecision 的 outcome 参数化形态（Q1）：装载/重复
// 拦截与既有守卫同用 outcome=blocked；wall-time / 轮数积软闸用
// outcome=tripped 落决策记录（once-per-entry）但不在该钩子拦截。
// plan-execute 漂移达阈值后由 BeforeTool 拦截（blocked）。
// toolName 为空时省略 tool 实体（软闸无单一触发工具）。
func (g *toolLoopGuard) emitGateEvent(ctx context.Context, outcome, toolName, scenario, reasoning string, observed any, threshold any, action string) {
	runID := gateRunID(ctx)
	var entities []decision.EntityRef
	if toolName != "" {
		entities = []decision.EntityRef{{Type: "tool", Key: toolName}}
	}
	event.EmitGate(ctx, g.decisions, decision.GateDecision{
		TriggerRule:   decision.TriggerLoopGuardBlocked,
		Outcome:       outcome,
		Scenario:      scenario,
		Reasoning:     reasoning,
		GuardName:     "tool_loop_guard",
		RunID:         runID,
		SessionID:     gateSessionID(ctx),
		Entities:      entities,
		ObservedValue: observed,
		Threshold:     threshold,
		Action:        action,
	})
}

// loopGuardInvocationKey 取守卫条目的隔离键：invocation + agent（=图谱节点身份）。
// 取不到时返回空串（守卫 fail-open，不影响工具执行）。
//
// 为什么带 AgentName（2026-08-16 复验实证「连坐」）：图谱执行时各节点共享
// run 级身份（彼时 InvocationID 全链路相同；2026-08-27 框架 Clone 补丁后成员
// 子 invocation 每次节点执行换新 uuid，run 级身份改经 ctx 注入的 team run id
// 表达，见下方优先取值），diagnose 第3步的 gns3_exec 取证与 remediate 调用1
// 同参同结果，计数跨节点累计 → remediate 首次调用即撞阈值被拦，真实取证额度
// 被压缩，模型「确认」反射未满足陷入重发死循环。按 AgentName 隔离后，各节点
// 独立享有完整计数额度；同节点跨执行仍共享（run id 不变），跨轮循环仍受控。
