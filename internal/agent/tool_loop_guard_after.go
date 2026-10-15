package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func (g *toolLoopGuard) afterHook() callbacks.AfterToolHook {
	return callbacks.NewAfterToolHook(4, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		// 注：被拦调用以 error 形态返回、框架不执行 AfterTool，故这里只见真实执行结果。
		key := loopGuardInvocationKey(ctx)
		// DEBUG(2026-09-02)：定位 inflightLoads 泄漏——确认 AfterTool 是否触发/key 是否为空，验证后移除。
		g.lg.Info("loop guard afterHook fired",
			loggateway.StepID("agent.tool_loop_guard"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Str("key", key),
			loggateway.Str("guard", fmt.Sprintf("%p", g)))
		if key == "" {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		sig := loopGuardSignature(args.ToolName, args.Arguments)
		// Q1 plan-execute 漂移观测的锁外发射负载（driftAt>0 且未发过才发射）。
		driftAt := 0
		g.mu.Lock()
		e := g.entryLocked(key, time.Now())
		e.endInflight(sig)
		if args.ToolName == toolLoadToolName {
			e.endInflightLoad(loopGuardToolLoadTarget(args.Arguments))
		}
		// A2'b 空转轮记账：本轮发生了真实工具调用。失败/检索空结果均不计
		// 产出（roundProductive 保持 false），BeforeModel 续轮结算空转轮次。
		e.roundSawTool = true
		if args.Error != nil {
			// 失败重试归熔断器治理：不累计重复计数，也不触发拦截。
			// FORBIDDEN/target not found 除外：同类失败 ≥2 次后拦下一跳（E3）。
			if loopGuardForbiddenNotFound(args.Error) && (e.lastSig == "" || e.lastSig == sig) {
				e.forbiddenStreak++
			} else if loopGuardForbiddenNotFound(args.Error) {
				e.forbiddenStreak = 1
			} else {
				e.forbiddenStreak = 0
			}
			e.noteErrorClass(args.ToolName, loopGuardErrorClass(args.ToolName, args.Result, args.Error))
			e.lastSig = sig
			e.lastTool = args.ToolName
			e.lastResultKey = ""
			e.sameCount = 0
			e.lastFailed = true
			e.appendCallLocked(sig, args.ToolName)
			g.mu.Unlock()
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		if class := loopGuardErrorClass(args.ToolName, args.Result, nil); class != "" {
			e.noteErrorClass(args.ToolName, class)
			e.lastSig = sig
			e.lastTool = args.ToolName
			e.lastResultKey = ""
			e.sameCount = 0
			e.lastFailed = true
			e.appendCallLocked(sig, args.ToolName)
			g.mu.Unlock()
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		e.clearErrorClass(args.ToolName)
		e.forbiddenStreak = 0
		rk := loopGuardResultKey(args.Result)
		sameResult := e.lastSig == sig && !e.lastFailed && e.lastResultKey == rk
		if e.lastSig == sig && !e.lastFailed && (sameResult || loopGuardVolatileResultTool(args.ToolName)) {
			e.sameCount++
		} else {
			e.sameCount = 1
		}
		e.lastSig = sig
		e.lastTool = args.ToolName
		e.lastResultKey = rk
		e.lastResultDigest = loopGuardResultDigest(args.Result)
		e.lastFailed = false
		// Q1 行为模式闸记账（成功路径）：
		//   - plan_and_execute 成功 → planDeclared 锚点置位；
		//   - tool_load 成功（result.success=true）→ 记 loadedTools（请求名+规范名，
		//     覆盖别名变体）；首次装载的目标才计 loadCount（配额只约束异质装载面）；
		//     锚点已置位时的成功装载累计 postPlanLoads，达阈值后 BeforeTool
		//     拦截下一次 tool_load；AfterTool 在达阈值当次写一次 tripped 记录。
		if args.ToolName == planAndExecuteToolName {
			e.planDeclared = true
		}
		if args.ToolName == toolLoadToolName {
			requested := loopGuardToolLoadTarget(args.Arguments)
			if name, ok := loopGuardToolLoadResult(args.Result, requested); ok {
				if e.loadedTools == nil {
					e.loadedTools = map[string]bool{}
				}
				if !e.loadedTools[name] {
					e.loadCount++
				}
				e.loadedTools[name] = true
				if requested != "" {
					e.loadedTools[requested] = true
				}
				e.markJustLoaded(name)
				if requested != "" {
					e.markJustLoaded(requested)
				}
				// DEBUG(2026-09-02)：误拦根因定位——记录 markJustLoaded 落点，验证后移除。
				g.lg.Info("loop guard markJustLoaded",
					loggateway.StepID("agent.tool_loop_guard"),
					loggateway.Str("tool", name),
					loggateway.Str("guard", fmt.Sprintf("%p", g)),
					loggateway.Str("key", key))
				if e.planDeclared {
					e.postPlanLoads++
					if e.postPlanLoads >= loopGuardPlanDriftObserveAt && !e.planDriftRecorded {
						e.planDriftRecorded = true
						driftAt = e.postPlanLoads
					}
				}
			}
		}
		// 空结果熔断记账（无视参数差异）：检索类工具连续空则累计，一旦拿到
		// 非空结果立即清零——熔断针对的是「库中确无资料仍换词重试」的空转。
		productive := true
		if isEmpty, ok := loopGuardEmptyResultTools[args.ToolName]; ok {
			if isEmpty(args.Result) {
				productive = false
				if e.emptyStreak == nil {
					e.emptyStreak = map[string]int{}
				}
				e.emptyStreak[args.ToolName]++
			} else {
				delete(e.emptyStreak, args.ToolName)
			}
		}
		if productive {
			e.roundProductive = true
		}
		e.appendCallLocked(sig, args.ToolName)
		g.mu.Unlock()
		if driftAt > 0 {
			g.lg.Warn("plan-execute drift observed: tool_load continues after plan declared",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Int("post_plan_loads", driftAt))
			g.emitGateEvent(ctx, "tripped", toolLoadToolName, "计划声明后仍持续本地装载工具（plan-execute 漂移）",
				fmt.Sprintf("plan_and_execute 已成功声明编排，节点随后又成功装载 %d 个工具——计划与执行漂移", driftAt),
				driftAt, loopGuardPlanDriftObserveAt, "record_only")
		}
		return &trpctool.AfterToolResult{Context: ctx}, nil
	})
}

// modelHook 是空转轮次早停（A2'b）与 wall-time 软闸（Q1）的 BeforeModel 侧：
// 每次模型调用（含工具循环续轮）先结算上一 LLM 轮的工具产出——
//   - 上一轮有工具调用且任一有产出 → unprodRounds 清零；
//   - 上一轮有工具调用但零有效产出（全部失败/被拦/检索空结果）→ unprodRounds+1；
//   - 上一轮无工具调用（纯文本续轮）→ 不结算，计数保持。
//
// 结算后 unprodRounds 满 unproductiveRoundGuideThreshold 即注入降级引导 cue
// （先摘除历史同名 cue，保证只有一条最新轮次文案）；满
// unproductiveRoundBlockThreshold 的封锁由 beforeHook 执行（拦截一切新调用，
// todo_declare_blocker 豁免）。
//
// Q1 wall-time 软闸：节点存续（entry.firstSeen 起算）超软闸秒数后，每轮注入
// 收尾引导 cue（与空转 cue 并存、各自按标记去重），并在首次越线时写一条
// outcome=tripped 决策记录（once-per-entry；cue 本身按轮刷新）。硬闸的
// StopError 强终止由 beforeHook 执行——模型侧先获引导、工具侧后封锁，与
// A2'b 两段式同构（对齐 LangGraph TimeoutPolicy idle/run 双超时语义）。
func (g *toolLoopGuard) modelHook() callbacks.BeforeModelHook {
	return callbacks.NewBeforeModelHook(4, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		key := loopGuardInvocationKey(ctx)
		// DEBUG(2026-09-02)：定位 justLoaded 跨 model step 未清除问题，验证后移除。
		g.lg.Info("loop guard model hook round",
			loggateway.StepID("agent.tool_loop_guard"),
			loggateway.Str("key", key),
			loggateway.Str("guard", fmt.Sprintf("%p", g)))
		if key == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		now := time.Now()
		g.mu.Lock()
		e := g.entryLocked(key, now)
		e.justLoaded = nil
		// inflightLoads 同属「单 model step 工具批次」窗口状态：BeforeModel 触发
		// 时上一轮全部工具调用必然已结束（框架须收齐结果才能构造下一轮请求），
		// 此处仍残留的条目只可能是 AfterTool 未触发（如插件 CustomResult 短路
		// 回调链）导致的泄漏。不清除会让目标工具在本节点内被 loadThenCall 闸
		// 永久误拦（2026-09-06 spirit WS 路径实锤：tool_load 成功后 afterHook
		// 未触发，inflightLoads 跨轮泄漏，twin_* 工具后续每轮都被拦截）。
		e.inflightLoads = nil
		if e.roundSawTool {
			if e.roundProductive {
				e.unprodRounds = 0
			} else {
				e.unprodRounds++
			}
			e.roundSawTool = false
			e.roundProductive = false
		}
		unprod := e.unprodRounds
		softSec := g.wallSecFor(false)
		elapsed := wallElapsedNet(ctx, e.firstSeen, now)
		wallSoftTripped := elapsed > time.Duration(softSec)*time.Second
		recordWallSoft := wallSoftTripped && !e.wallSoftRecorded
		if recordWallSoft {
			e.wallSoftRecorded = true
		}
		e.modelRounds++
		e.lastEst = analyzePromptRequest(args.Request.Messages).EstTokens + toolsSchemaEstTokens(args.Request)
		prod := e.modelRounds * e.lastEst
		softProd := g.roundProductSoft()
		hardProd := g.roundProductHard()
		prodSoftTripped := hardProd > 0 && prod >= softProd
		recordProdSoft := prodSoftTripped && !e.roundProdRecorded
		if recordProdSoft {
			e.roundProdRecorded = true
		}
		rounds := e.modelRounds
		est := e.lastEst
		g.mu.Unlock()
		msgs := args.Request.Messages
		if unprod >= unproductiveRoundGuideThreshold {
			msgs = stripDynamicCueByMarker(msgs, unproductiveRoundCueMarker)
			msgs = appendDynamicCue(msgs, unproductiveRoundCueMarker+buildUnproductiveRoundCue(unprod))
			g.lg.Warn("unproductive tool rounds, degradation guide injected",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Int("unproductive_rounds", unprod))
		}
		if wallSoftTripped {
			msgs = stripDynamicCueByMarker(msgs, wallTimeCueMarker)
			msgs = appendDynamicCue(msgs, wallTimeCueMarker+buildWallTimeCue(elapsed, g.wallSecFor(true)))
			g.lg.Warn("wall-time soft gate tripped, wrap-up cue injected",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Int("elapsed_sec", int(elapsed.Seconds())),
				loggateway.Int("soft_sec", softSec))
			if recordWallSoft {
				g.emitGateEvent(ctx, "tripped", "", "节点运行超 wall-time 软闸，注入收尾引导",
					fmt.Sprintf("节点已运行 %d 秒，超过 wall-time 软闸 %d 秒，注入收尾引导 cue", int(elapsed.Seconds()), softSec),
					int(elapsed.Seconds()), softSec, "inject_cue")
			}
		}
		if prodSoftTripped {
			msgs = stripDynamicCueByMarker(msgs, roundProductCueMarker)
			msgs = appendDynamicCue(msgs, roundProductCueMarker+buildRoundProductCue(rounds, est, prod, hardProd))
			g.lg.Warn("round×est product soft gate tripped, wrap-up cue injected",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Int("model_rounds", rounds),
				loggateway.Int("last_est", est),
				loggateway.Int("product", prod),
				loggateway.Int("soft", softProd),
				loggateway.Int("hard", hardProd))
			if recordProdSoft {
				g.emitGateEvent(ctx, "tripped", "", "轮数×单轮积越软闸，注入收尾引导",
					fmt.Sprintf("%d 轮 × %d tok = %d，软阈 %d / 硬阈 %d", rounds, est, prod, softProd, hardProd),
					prod, softProd, "inject_cue")
			}
		}
		args.Request.Messages = msgs
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// buildUnproductiveRoundCue 是降级引导 cue 文案（A2'b）：告知模型已连续 N 轮
// 零有效产出，引导其停止盲目重试、基于已有信息收尾；满封锁阈值时改口告知
// 工具面已封锁（与 beforeHook 拦截文案口径一致），仅剩 declare_blocker/收尾。
func buildUnproductiveRoundCue(unprod int) string {
	if unprod >= unproductiveRoundBlockThreshold {
		return fmt.Sprintf(`<tool_round_notice>
你已连续 %d 轮工具调用零有效产出（失败/被系统拦截/检索空结果）。系统现已封锁新的工具调用：除 todo_declare_blocker 外，任何工具调用都会被直接拦截。不要再尝试调用工具——立即基于现有信息输出最终结论；若任务确实无法推进，调用 todo_declare_blocker 说明阻塞原因后收尾。
</tool_round_notice>`, unprod)
	}
	return fmt.Sprintf(`<tool_round_notice>
你已连续 %d 轮工具调用零有效产出（失败/被系统拦截/检索空结果）。继续用相似方式调用工具大概率仍无收获，只会消耗调用预算。请立即调整策略：基于已拿到的信息直接作答；或换用根本不同的方法（不同工具/不同思路）。若确认任务无法推进，调用 todo_declare_blocker 说明阻塞原因。再连续零产出 %d 轮，系统将封锁一切新的工具调用。
</tool_round_notice>`, unprod, unproductiveRoundBlockThreshold-unprod)
}

// buildWallTimeCue 是 wall-time 软闸的收尾引导 cue 文案（Q1）：告知模型节点
// 已运行时长与硬时限，引导其停止扩张性动作（继续装载/广泛取证）、基于已
// 有信息收尾。硬闸强终止由 beforeHook 执行，文案口径与 StopError 消息一致。
func buildWallTimeCue(elapsed time.Duration, hardSec int) string {
	return fmt.Sprintf(`<runtime_notice>
本节点已运行约 %d 分钟，超过系统软时限；运行满 %d 秒将被强制终止（不可豁免）。请立即停止扩张性动作（装载新工具/广泛取证/轮询等待），基于已取得的信息输出当前最优结论收尾；若任务确实无法完成，说明缺口后收尾，不要继续消耗预算。
</runtime_notice>`, int(elapsed.Minutes()), hardSec)
}

func buildRoundProductCue(rounds, est, prod, hard int) string {
	return fmt.Sprintf(`<runtime_notice>
本节点已进行 %d 次模型调用，最近一轮约 %d tokens（轮数×单轮积 %d）。继续调用工具会按轮数平方放大计费；积达到 %d 后系统将封锁新的工具调用。请立即停止扩张性装载/检索，基于已有信息输出结论；若任务无法完成，调用 todo_declare_blocker 说明缺口后收尾。
</runtime_notice>`, rounds, est, prod, hard)
}

// loopGuardVolatileResultTool reports tools whose success payload always
// changes (clock/time) so same-param repeats would never trip the
// result-hash guard. Same signature still counts toward the 3rd-call block.
func loopGuardVolatileResultTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "datetime", "get_datetime", "current_time", "get_current_time", "now", "clock":
		return true
	default:
		return false
	}
}

func loopGuardForbiddenNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "forbidden") && strings.Contains(msg, "target agent not found")
}
