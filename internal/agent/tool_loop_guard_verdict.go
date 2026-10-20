package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/tools/inverse"
	"aranea-agents/pkg/loggateway"
)

func (g *toolLoopGuard) verdictBeforeLocked(e *loopGuardEntry, sig, toolName string, args []byte, now time.Time) loopGuardVerdict {
	if e.inflightCount(sig, now) > 0 {
		return loopGuardVerdict{kind: loopGuardBlockParallel}
	}
	// P2-③ HITL 恢复路径（R4-Q6 根修）：本签名刚被确认门禁否决（用户拒绝/
	// 无回复通道，inflight 已由 noteConfirmationOutcome 归还）——同一节点
	// 生命周期内同参重发即拦。被拦调用不进 AfterTool，blockedCount 与本轮
	// 工具活动只能在此记账（与下方通用拦截路径同构）。
	if e.deniedSigs[sig] > 0 {
		e.blockedCount++
		e.roundSawTool = true
		return loopGuardVerdict{
			kind:         loopGuardBlockDenied,
			blockedCount: e.blockedCount,
			saturated:    e.blockedCount >= loopGuardSaturatedStopThreshold,
		}
	}
	// Q1 装载闸（S02「合法失控」根修）：优先于签名/轮换/空结果判定——
	// tool_load 的异质调用序列对三重旧守卫全部"合法"，必须由装载语义自身收口。
	//   - 重复装载：目标已在 loadedTools（激活幂等、无运行时卸载，重复装载恒零
	//     新信息），第二次起拦截，引导模型直接调用已激活工具；
	//   - 配额：异质装载面 loadCount 达阈值（默认 8，DB 可覆盖）后拦截新装载——
	//     装载行为本身即失控向量（S02 观测 24 次异质装载），与装载结果成功与否无关。
	//   - 占比闸：窗口内 tool_load/tool_search ≥50% 封锁新自举（C1，异质装载
	//     在配额耗尽前收敛）。
	//   - 同批直调：刚 tool_load 的目标不能在同一 model step / 并行批次里立刻 call。
	// 参数不可解析时跳过装载闸（fail-open，参数错误归工具自身报错）。
	var loadTarget string
	loadRepeat := false
	loadQuota := false
	loadRatio := false
	loadThenCall := ""
	loadMax := 0
	if toolName == toolLoadToolName {
		loadTarget = loopGuardToolLoadTarget(args)
		if loadTarget != "" {
			loadMax = g.loadMaxFor()
			if e.loadedTools[loadTarget] {
				loadRepeat = true
			} else if e.loadCount >= loadMax {
				loadQuota = true
			}
		}
	}
	if !loadRepeat && !loadQuota && loopGuardBootstrapRatioTripped(e.recentTools, toolName, g.bootstrapRatioFor()) {
		loadRatio = true
		if loadTarget == "" {
			loadTarget = toolName
		}
	}
	planDrift := false
	if !loadRepeat && !loadQuota && !loadRatio {
		if at := g.planDriftAt(); at > 0 && e.planDeclared && e.postPlanLoads >= at && toolName == toolLoadToolName {
			planDrift = true
			if loadTarget == "" {
				loadTarget = loopGuardToolLoadTarget(args)
			}
		}
	}
	if !loadRepeat && !loadQuota && !loadRatio {
		loadThenCall = e.loadThenCallTarget(toolName)
		// DEBUG(2026-09-02)：误拦根因定位——dump 两张表与 guard 实例指针，验证后移除。
		if loadThenCall != "" {
			g.lg.Info("loop guard loadThenCall hit",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", toolName),
				loggateway.Str("target", loadThenCall),
				loggateway.Str("guard", fmt.Sprintf("%p", g)),
				loggateway.Str("inflight_loads", fmt.Sprintf("%v", e.inflightLoads)),
				loggateway.Str("just_loaded", fmt.Sprintf("%v", e.justLoaded)))
		}
	}
	// 空结果熔断优先于签名/轮换判定：库中确无资料时，任何参数的再调用都无意义，
	// 拦截消息也比通用重复文案更具行动指引（直接作答/声明未收录）。
	emptyBlocked := e.emptyStreak[toolName] >= loopGuardEmptyStreakThreshold
	errorClassBlocked := e.errorClassStreak[toolName] >= loopGuardErrorClassStreakThreshold
	// P2-③ 补偿对阈值收紧（R4-Q6 根修）：副作用工具的副作用不可重放——
	// 重复 inject 不叠加新效果、重复 clear 幂等空转，首次成功即视为终态，
	// 第 2 次同参同结果调用起拦（含审批通过后的重放形态）；其余工具维持
	// 默认阈值（取证确认属合理模式）。
	loopThreshold := loopGuardBlockThreshold
	if loopGuardCompensationPairTool(toolName) || loopGuardReadOnlyRetrievalTool(toolName) {
		loopThreshold = 1
	}
	loop := e.lastSig == sig && !e.lastFailed && e.sameCount >= loopThreshold
	forbiddenLoop := e.lastSig == sig && e.forbiddenStreak >= 2
	var cycleDesc string
	if !loop && len(e.recentSigs)+1 >= 2*loopGuardCycleMinRepeats {
		// 试追加当前签名，检测末尾是否构成固定轮换循环（如 A→B→C 满 3 轮）。
		trialSigs := append(append([]string(nil), e.recentSigs...), sig)
		if p := loopGuardCyclePeriod(trialSigs, loopGuardCycleMinRepeats); p >= 2 {
			trialTools := append(append([]string(nil), e.recentTools...), toolName)
			cycleDesc = strings.Join(trialTools[len(trialTools)-p:], " → ")
		}
	}
	if !loadRepeat && !loadQuota && !loadRatio && !planDrift && loadThenCall == "" && !emptyBlocked && !errorClassBlocked && !loop && !forbiddenLoop && cycleDesc == "" {
		e.beginInflight(sig, now)
		if toolName == toolLoadToolName && loadTarget != "" {
			e.beginInflightLoad(loadTarget)
		}
		return loopGuardVerdict{kind: loopGuardBlockNone}
	}
	// B4 饱和止损：被拦调用不进 AfterTool，计数只能在此累计。
	e.blockedCount++
	// A2'b：被拦调用不进 AfterTool，本轮工具活动只能在此记账（被拦=零产出，
	// roundProductive 保持 false，供 BeforeModel 结算空转轮次）。
	e.roundSawTool = true
	v := loopGuardVerdict{
		blockedCount:     e.blockedCount,
		consecutiveCount: e.sameCount,
		emptyStreak:      e.emptyStreak[toolName],
		forbiddenStreak:  e.forbiddenStreak,
		errorClass:       e.lastErrorClass[toolName],
		errorClassN:      e.errorClassStreak[toolName],
		lastDigest:       e.lastResultDigest,
		lastTool:         e.lastTool,
		loadTarget:       loadTarget,
		loadCount:        e.loadCount,
		loadMax:          loadMax,
		saturated:        e.blockedCount >= loopGuardSaturatedStopThreshold,
		threshold:        loopThreshold,
	}
	if loadThenCall != "" {
		v.kind = loopGuardBlockLoadThenCall
		v.loadTarget = loadThenCall
		return v
	}
	if loadRepeat {
		v.kind = loopGuardBlockLoadRepeat
		return v
	}
	if loadQuota {
		v.kind = loopGuardBlockLoadQuota
		return v
	}
	if loadRatio {
		v.kind = loopGuardBlockLoadRatio
		return v
	}
	if planDrift {
		v.kind = loopGuardBlockPlanDrift
		v.loadCount = e.postPlanLoads
		return v
	}
	if emptyBlocked {
		v.kind = loopGuardBlockEmpty
		return v
	}
	if errorClassBlocked {
		v.kind = loopGuardBlockErrorClass
		return v
	}
	if cycleDesc != "" {
		v.kind = loopGuardBlockCycle
		v.cycleDesc = cycleDesc
		return v
	}
	if forbiddenLoop {
		v.kind = loopGuardBlockForbidden
		return v
	}
	v.kind = loopGuardBlockLoop
	return v
}

// loopGuardCompensationPairTool 报告工具是否属于已登记的补偿对（正向或逆向）。
// 补偿对工具的副作用不可重放（重复 inject 不叠加新效果、重复 clear 幂等空转），
// 同参去重阈值收紧为 1（P2-③，R4-Q6 根修：审批通过后的同参重放首次即拦）。
// 注册表为进程级只读快照（构造路径幂等注册），锁外调用安全。
func loopGuardCompensationPairTool(toolName string) bool {
	if _, ok := inverse.LookupForward(toolName); ok {
		return true
	}
	return inverse.IsInverse(toolName)
}

// loopGuardReadOnlyRetrievalTool 报告工具是否为只读检索类（P6-N8）。
// 只读检索不产生副作用，同参重复调用不会返回新信息——阈值收紧为 1
// （同节点内第 2 次同参同结果调用即拦），避免空转烧 token（S02 实证：
// memory_search 18 秒内同参重复 2 次结果相同）。
func loopGuardReadOnlyRetrievalTool(toolName string) bool {
	switch toolName {
	case "memory_search", "knowledge_search", "list_inbox", "tool_search":
		return true
	}
	return false
}

// noteConfirmationOutcome 由 HITL 确认门禁在非批准出口调用（P2-③，R4-Q6 根修）。
//
// 背景：确认门禁（priority 10）在循环守卫（priority 4）放行后才等待用户裁定，
// 守卫已为该签名 beginInflight；非批准出口下工具不再执行、框架短路返回
// CustomResult、AfterTool 不运行，inflight 槽位必须由本函数显式归还——否则
// 陈旧期（loopGuardInflightStale）内的重发会被误判为「并行重复」。
//
// denied=true（用户明确拒绝 / 无回复通道）额外把签名登记进 deniedSigs：同一
// 节点生命周期内同参重发即拦，拦截消息由 beforeHook 拼接方案 C 补偿引导。
// denied=false（确认超时 / 等待异常）只归还槽位不登记——确认卡仍然有效，
// 用户授意的重发必须能再次走进确认流程。
//
// 批准出口严禁调用本函数：工具继续执行，inflight 由 AfterTool 正常归还，
// 重复归还会吞掉后续真实并行调用的计数。
func (g *toolLoopGuard) noteConfirmationOutcome(ctx context.Context, toolName string, args []byte, denied bool) {
	if g == nil {
		return
	}
	key := loopGuardInvocationKey(ctx)
	if key == "" {
		return
	}
	sig := loopGuardSignature(toolName, args)
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.entryLocked(key, time.Now())
	e.endInflight(sig)
	if toolName == toolLoadToolName {
		e.endInflightLoad(loopGuardToolLoadTarget(args))
	}
	if denied {
		if e.deniedSigs == nil {
			e.deniedSigs = map[string]int{}
		}
		e.deniedSigs[sig]++
	}
	// 本轮有工具活动但零产出（工具未执行），供 BeforeModel 空转轮结算。
	e.roundSawTool = true
}
