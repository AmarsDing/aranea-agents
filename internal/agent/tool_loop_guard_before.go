package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func (g *toolLoopGuard) beforeHook() callbacks.BeforeToolHook {
	return callbacks.NewBeforeToolHook(4, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		key := loopGuardInvocationKey(ctx)
		if key == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		sig := loopGuardSignature(args.ToolName, args.Arguments)
		now := time.Now()
		g.mu.Lock()
		e := g.entryLocked(key, now)
		// Q1 wall-time 硬闸：节点存续超过硬时限 → StopError 强制终止（S02
		// 「合法失控」根修；对齐 LangGraph TimeoutPolicy 的 run 级硬超时）。
		// 无豁免工具——硬闸语义是节点生命期终结，不同于空转封锁保留投降通道。
		// 计时基准 firstSeen=条目创建（节点在本 run 内首次活动），跨重试累计：
		// 节点反复重跑仍持续烧预算正是硬闸要拦的形态。
		if hardSec := g.wallSecFor(true); wallElapsedNet(ctx, e.firstSeen, now) > time.Duration(hardSec)*time.Second {
			elapsed := wallElapsedNet(ctx, e.firstSeen, now)
			g.mu.Unlock()
			g.lg.Warn("tool loop guard wall-time hard gate tripped, stopping node",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("elapsed_sec", int(elapsed.Seconds())),
				loggateway.Int("hard_sec", hardSec))
			g.emitGateEvent(ctx, "blocked", args.ToolName, "节点运行超 wall-time 硬闸，强制终止",
				fmt.Sprintf("节点已运行 %d 秒，超过 wall-time 硬闸 %d 秒", int(elapsed.Seconds()), hardSec),
				int(elapsed.Seconds()), hardSec, "stop_node")
			return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已运行约 %d 分钟，超过系统硬时限（%d 秒），"+
				"节点被强制终止以防止预算耗尽。已取得的结论与本次终止原因将随节点结果上报；后续步骤由编排层接手。",
				loopGuardMarker, int(elapsed.Minutes()), hardSec))
		}
		if hard := g.roundProductHard(); hard > 0 {
			prod := e.modelRounds * e.lastEst
			if prod >= hard && args.ToolName != todoDeclareBlockerToolName {
				e.blockedCount++
				e.roundSawTool = true
				blocked := e.blockedCount
				rounds := e.modelRounds
				est := e.lastEst
				saturated := blocked >= loopGuardSaturatedStopThreshold
				g.mu.Unlock()
				g.lg.Warn("tool loop guard blocked call after round×est product hard gate",
					loggateway.StepID("agent.tool_loop_guard"),
					loggateway.Str("tool", args.ToolName),
					loggateway.Int("model_rounds", rounds),
					loggateway.Int("last_est", est),
					loggateway.Int("product", prod),
					loggateway.Int("hard", hard))
				g.emitGateDecision(ctx, args.ToolName, "轮数×单轮积越硬闸，工具面封锁",
					fmt.Sprintf("%d 轮 × %d tok = %d，超过硬阈 %d", rounds, est, prod, hard),
					prod, hard, "block_call")
				if saturated {
					return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已连续 %d 次触发系统拦截仍重发被拦调用，"+
						"节点被强制终止以防止调用预算耗尽。已取得的取证结论与本次终止原因将随节点结果上报。", loopGuardMarker, loopGuardSaturatedStopThreshold))
				}
				msg := fmt.Sprintf("%s：本节点已进行 %d 次模型调用，最近一轮约 %d tokens（积 %d，硬阈 %d）——本调用被系统拦截，工具未执行、也非执行失败。"+
					"继续调用只会按轮数平方放大计费。请立即基于现有信息输出最终结论；若任务无法推进，调用 todo_declare_blocker 说明阻塞原因。"+
					"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
					loopGuardMarker, rounds, est, prod, hard, blocked, loopGuardSaturatedStopThreshold)
				return nil, errors.New(msg)
			}
		}
		// A2'b 空转封锁：连续 M 轮零有效产出后，封锁一切新工具调用
		// （todo_declare_blocker 豁免，保留投降通道）。顽固重发计入
		// blockedCount，共享 B4 饱和 StopError 止损。
		if e.unprodRounds >= unproductiveRoundBlockThreshold && args.ToolName != todoDeclareBlockerToolName {
			e.blockedCount++
			e.roundSawTool = true
			blocked := e.blockedCount
			unprod := e.unprodRounds
			saturated := blocked >= loopGuardSaturatedStopThreshold
			g.mu.Unlock()
			g.lg.Warn("tool loop guard blocked call after unproductive rounds",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("unproductive_rounds", unprod),
				loggateway.Int("blocked", blocked))
			action := "block_call"
			if saturated {
				action = "stop_node"
			}
			g.emitGateDecision(ctx, args.ToolName, "连续零有效产出轮，工具面封锁",
				fmt.Sprintf("连续 %d 轮工具调用零有效产出，封锁新调用（%s）", unprod, action),
				unprod, unproductiveRoundBlockThreshold, action)
			if saturated {
				return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已连续 %d 次触发系统拦截仍重发被拦调用，"+
					"节点被强制终止以防止调用预算耗尽。已取得的取证结论与本次终止原因将随节点结果上报。", loopGuardMarker, loopGuardSaturatedStopThreshold))
			}
			msg := fmt.Sprintf("%s：本节点已连续 %d 轮工具调用零有效产出（失败/被拦/检索空结果），系统已封锁新的工具调用——本调用未执行、也非执行失败。"+
				"继续调用任何工具都不会产生新信息。请立即基于现有信息输出最终结论；若任务确实无法推进，调用 todo_declare_blocker 说明阻塞原因（该工具不受本封锁限制）。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, unproductiveRoundBlockThreshold, blocked, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		v := g.verdictBeforeLocked(e, sig, args.ToolName, args.Arguments, now)
		g.mu.Unlock()
		switch v.kind {
		case loopGuardBlockNone:
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		case loopGuardBlockParallel:
			g.lg.Warn("tool loop guard blocked parallel duplicate call",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName))
			msg := fmt.Sprintf("%s：同一轮并行发出了重复的 %s 调用，仅执行第一次。工具未执行、也非执行失败。"+
				"禁止在同一响应中重复发射相同工具与相同参数；请等待第一次结果后再决定下一步。",
				loopGuardMarker, args.ToolName)
			return nil, errors.New(msg)
		}
		if v.saturated {
			g.lg.Warn("tool loop guard saturated, stopping node",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("blocked", loopGuardSaturatedStopThreshold))
			g.emitGateDecision(ctx, args.ToolName, "拦截饱和，节点强制终止",
				fmt.Sprintf("节点内累计被拦 %d 次仍重发被拦调用，强制终止节点止损", loopGuardSaturatedStopThreshold),
				loopGuardSaturatedStopThreshold, loopGuardSaturatedStopThreshold, "stop_node")
			return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已连续 %d 次触发系统拦截仍重发被拦调用，"+
				"节点被强制终止以防止调用预算耗尽。已取得的取证结论与本次终止原因将随节点结果上报。", loopGuardMarker, loopGuardSaturatedStopThreshold))
		}
		if v.kind == loopGuardBlockLoadRepeat {
			g.lg.Warn("tool loop guard blocked repeated tool_load",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("target", v.loadTarget))
			g.emitGateDecision(ctx, args.ToolName, "重复装载已激活工具，拦截零信息调用",
				fmt.Sprintf("工具 %s 已处于激活状态，重复 tool_load 不产生新信息", v.loadTarget),
				v.loadTarget, 1, "block_call")
			msg := fmt.Sprintf("%s：工具 %s 此前已成功装载并处于激活状态——激活是幂等的，本调用被系统拦截，工具未执行、也非执行失败。"+
				"重复装载不会产生任何新信息（schema 已在历史中，且本会话无卸载机制）。请立即直接调用 %s 本身推进任务，或基于现有信息收尾。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.loadTarget, v.loadTarget, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockLoadQuota {
			g.lg.Warn("tool loop guard blocked tool_load over quota",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("target", v.loadTarget),
				loggateway.Int("load_count", v.loadCount),
				loggateway.Int("load_max", v.loadMax))
			g.emitGateDecision(ctx, args.ToolName, "工具装载配额耗尽，拦截装载面扩张",
				fmt.Sprintf("节点已成功装载 %d 个工具，达配额上限 %d，继续装载属失控模式", v.loadCount, v.loadMax),
				v.loadCount, v.loadMax, "block_call")
			msg := fmt.Sprintf("%s：本节点已成功装载 %d 个工具，达到单节点装载配额上限（%d）——本调用被系统拦截，工具未执行、也非执行失败。"+
				"持续装载新工具而不使用是典型的失控模式：请立即停止 tool_load，从已激活的工具中选择能推进任务的直接调用；若现有工具面确实无法完成，基于已有信息说明缺口后收尾。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.loadCount, v.loadMax, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockLoadRatio {
			g.lg.Warn("tool loop guard blocked bootstrap ratio",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("target", v.loadTarget))
			g.emitGateDecision(ctx, args.ToolName, "装载/搜索占比过高，强制收敛",
				fmt.Sprintf("窗口内 tool_load/tool_search 占比达到 %.0f%%，继续自举属失控模式", loopGuardBootstrapRatioDefault*100),
				loopGuardBootstrapWindow, int(loopGuardBootstrapRatioDefault*100), "block_call")
			msg := fmt.Sprintf("%s：本节点最近工具调用中 tool_load/tool_search 占比已达 50%% 以上——本调用被系统拦截，工具未执行、也非执行失败。"+
				"持续搜索/装载而不使用是 S02 类失控：请立即停止 tool_load 与 tool_search，直接调用已激活工具推进任务；若工具面不足，基于已有信息说明缺口后收尾。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockPlanDrift {
			g.lg.Warn("tool loop guard blocked plan-execute drift tool_load",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("target", v.loadTarget),
				loggateway.Int("post_plan_loads", v.loadCount))
			g.emitGateDecision(ctx, args.ToolName, "计划声明后持续本地装载，拦截漂移",
				fmt.Sprintf("plan_and_execute 已声明编排后仍装载 %d 个工具", v.loadCount),
				v.loadCount, g.planDriftAt(), "block_call")
			msg := fmt.Sprintf("%s：本节点已通过 plan_and_execute 声明编排，随后又本地装载了 %d 个工具——本调用被系统拦截，工具未执行、也非执行失败。"+
				"计划与执行漂移：编排既已声明，应等待成员结果或基于已有信息收尾，不要继续 tool_load。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.loadCount, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockLoadThenCall {
			g.lg.Warn("tool loop guard blocked same-step call after tool_load",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("target", v.loadTarget),
				loggateway.Str("key", key))
			g.emitGateDecision(ctx, args.ToolName, "刚激活的工具不能在同一步立刻调用",
				fmt.Sprintf("工具 %s 刚由 tool_load 激活，须等下一 model step", v.loadTarget),
				v.loadTarget, 1, "block_call")
			msg := fmt.Sprintf("%s：工具 %s 刚在本轮由 tool_load 激活（或与 tool_load 处于同一批并行调用）——本调用被系统拦截，工具未执行、也非执行失败。"+
				"请等待 tool_load 返回后，在下一轮模型请求中调用 %s，不要与 tool_load 放在同一批并行 tool call 里。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.loadTarget, v.loadTarget, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockEmpty {
			g.lg.Warn("tool loop guard blocked empty-result retry",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("empty_streak", v.emptyStreak))
			g.emitGateDecision(ctx, args.ToolName, "检索连续空结果，拦截换词重试",
				fmt.Sprintf("%s 连续 %d 次返回空结果（含换词），库中确无资料", args.ToolName, v.emptyStreak),
				v.emptyStreak, loopGuardEmptyStreakThreshold, "block_call")
			msg := fmt.Sprintf("%s：%s 已连续 %d 次（含换用不同关键词）返回空结果——知识库中确实没有相关资料。"+
				"本调用被系统拦截，工具未执行、也非执行失败；继续换词重试不会产生任何新信息，只会消耗你的调用预算。"+
				"请立即停止调用本工具。若本轮已注入 ## L2+L3 memory，必须根据其中匹配事实作答，禁止声称记忆中没有。"+
				"若记忆与知识库都没有该事实，在回答中明确说明未收录，禁止编造姓名、报告编号、品牌、电话或个人偏好。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, args.ToolName, v.emptyStreak, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockErrorClass {
			g.lg.Warn("tool loop guard blocked same-error-class retry",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("error_class", v.errorClass),
				loggateway.Int("error_class_streak", v.errorClassN))
			g.emitGateDecision(ctx, args.ToolName, "同类业务错误连续失败，拦截重试",
				fmt.Sprintf("%s 连续 %d 次返回同类错误 %s", args.ToolName, v.errorClassN, v.errorClass),
				v.errorClassN, loopGuardErrorClassStreakThreshold, "block_call")
			hint := "请改用正确参数、换工具，或基于已有失败信息收尾，不要原样重试。"
			if v.errorClass == "empty_command" {
				hint = "命令参数名是 cmd（也接受 command）；空命令不会成功。请填入只读白名单命令，或停止调用本工具。"
			}
			msg := fmt.Sprintf("%s：%s 已连续 %d 次返回同类错误（%s）——本调用被系统拦截，工具未执行。%s"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, args.ToolName, v.errorClassN, v.errorClass, hint,
				v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockCycle {
			g.lg.Warn("tool loop guard blocked rotation cycle call",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("cycle", v.cycleDesc))
			g.emitGateDecision(ctx, args.ToolName, "固定调用循环，拦截轮询空转",
				fmt.Sprintf("检测到固定调用循环（%s），已重复满 %d 轮", v.cycleDesc, loopGuardCycleMinRepeats),
				v.cycleDesc, loopGuardCycleMinRepeats, "block_call")
			msg := fmt.Sprintf("%s：检测到固定调用循环（%s），已重复满 %d 轮——本调用被系统拦截，工具未执行、也非执行失败，"+
				"节点内轮询不会产生新信息。若在等待外部状态变化，状态复验由图谱重试机制承担，禁止继续该循环；请立即基于现有证据输出结论/裁决。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.cycleDesc, loopGuardCycleMinRepeats, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockForbidden {
			g.lg.Warn("tool loop guard blocked forbidden retry",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("forbidden_streak", v.forbiddenStreak))
			g.emitGateDecision(ctx, args.ToolName, "目标 agent 不存在，停止重试",
				fmt.Sprintf("%s 连续返回 FORBIDDEN/target not found，继续重试不会接通", args.ToolName),
				2, 2, "block_call")
			msg := fmt.Sprintf("%s：%s 已连续因目标 agent 不存在（FORBIDDEN）失败——本调用被系统拦截。"+
				"不要再猜 agent_key 或重试同一调用。请改走路由建议（部门主管会话/精灵组队）或如实向用户说明岗位未接通；禁止继续 memory_search 打转。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, args.ToolName, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockDenied {
			g.lg.Warn("tool loop guard blocked HITL-denied resend",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName))
			g.emitGateDecision(ctx, args.ToolName, "同参重发已被确认门禁否决的调用，拦截",
				fmt.Sprintf("%s 的同参调用刚被确认流程否决（用户拒绝或无确认通道），节点内重发不会改变裁定", args.ToolName),
				1, 1, "block_call")
			cue := planCCompensationCue(ctx, args.ToolName)
			if cue != "" {
				cue = " " + cue
			}
			msg := fmt.Sprintf("%s：本调用与方才确认流程已否决的调用（工具与参数完全一致）重合——该调用已被判定为不可执行（用户拒绝或当前环境无确认通道），本调用被系统拦截，工具未执行、也非执行失败。"+
				"同一节点内以相同参数重发不会改变裁定；若用户改变主意，会在新的对话轮次中明确提出，届时重新发起确认。请直接向用户说明该操作保持未执行状态，并询问接下来如何处理。%s"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, cue, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		threshold := v.threshold
		if threshold <= 0 {
			threshold = loopGuardBlockThreshold
		}
		g.lg.Warn("tool loop guard blocked identical repeat call",
			loggateway.StepID("agent.tool_loop_guard"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Int("consecutive", v.consecutiveCount))
		g.emitGateDecision(ctx, args.ToolName, "同工具同参数同结果连续重发，拦截无效空转",
			fmt.Sprintf("%s 已连续 %d 次以相同参数调用且结果一致（第 %d 次起拦截）", args.ToolName, v.consecutiveCount, threshold+1),
			v.consecutiveCount, threshold+1, "block_call")
		digest := v.lastDigest
		if digest == "" {
			digest = "（结果为空）"
		}
		crossName := ""
		if v.lastTool != "" && v.lastTool != args.ToolName {
			crossName = fmt.Sprintf("（%s 与 %s 是同一底层工具的不同名字，同参数调用属重复）", args.ToolName, v.lastTool)
		}
		// P2-③ 方案 C 引导：补偿对工具（如 fault_inject）存在未核销副作用时，
		// 把模型从「重发原调用」引导到「调用逆工具完成补偿」；无 pending 时为空串。
		cue := planCCompensationCue(ctx, args.ToolName)
		if cue != "" {
			cue = " " + cue
		}
		msg := fmt.Sprintf("%s：本调用被系统拦截，工具未执行、也非执行失败——%s 此前已成功返回，取证已完成。%s"+
			"禁止重发本调用，立即按任务指令推进到下一动作（发起下一步指定的工具调用；全部步骤完成则直接输出最终结论）。"+
			"重发只会反复触发本拦截并消耗你的调用预算，不会产生任何新信息。完整取证结果回放：「%s」（你已连续 %d 次以相同参数调用；本节点累计被拦 %d 次，满 %d 次将被强制终止）。%s",
			loopGuardMarker, args.ToolName, crossName, digest, v.consecutiveCount, v.blockedCount, loopGuardSaturatedStopThreshold, cue)
		return nil, errors.New(msg)
	})
}
