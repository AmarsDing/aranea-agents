package team

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// teamTurnBaseRunOptions builds the base run options for a team turn.
// C5: team_id is injected into the root invocation's RuntimeState; the graph
// runtime propagates it to member invocations (merged into graph initial
// state, then carried into each member's RunOptions.RuntimeState), enabling
// team-scope L3 memory recall via memoryRuntimeContext's RuntimeState fallback.
func teamTurnBaseRunOptions(teamID, turnQuery string) []trpcagent.RunOption {
	return []trpcagent.RunOption{
		skillruntime.RunOptionWithTurnQuery(turnQuery),
		trpcagent.MergeRuntimeState(map[string]any{"team_id": teamID}),
		// 框架 v1.11 修复管线：参数 JSON 修复 + 文本工具调用提取（与
		// chat 主路径 buildTurnRunOptions 对齐）。
		trpcagent.WithToolCallArgumentsJSONRepairEnabled(true),
		trpcagent.WithToolCallTextRepairEnabled(true),
		// 2026-09-01 错误传播口径统一（方案 B）：成员终态错误（LLM 流
		// 被掐等）上抛为 graph 节点失败，team_run_steps 记 error 与
		// steps_v2 证据同口径，团队 FailurePolicy 即时生效。配套必要
		// 条件：provider 活性守卫装饰器对错误响应做 Object 归一化
		// （框架 isTerminalAgentErrorEvent 的 Object 门）。
		trpcagent.WithPropagateChildAgentErrors(true),
	}
}

// resolveUpstreamDeliverableSeed resolves the cross-team deliverable seed for
// a DAG downstream team turn (2026-08-08 问题3c). Returns nil when the runner
// has no seed resolver wired, the team is standalone (no DagNodeID), or the
// team has no DependsOn. Seed read failure degrades to nil with a Warn — a
// seed error must never fail the turn (members still have the prompt-inlined
// digests and the read_upstream_deliverable tool as fallback).
func (r *Runner) resolveUpstreamDeliverableSeed(ctx context.Context, teamRow biz.Team) map[string]any {
	if r.upstreamSeedFn == nil || teamRow.DagNodeID == "" || len(teamRow.DependsOn) == 0 {
		return nil
	}
	seed, err := r.upstreamSeedFn(ctx, teamRow)
	if err != nil {
		r.lg.Warn("上游交付物种子解析失败，降级为无种子启动",
			loggateway.StepID("team.run.upstream_seed"),
			loggateway.Str("team_id", teamRow.ID),
			loggateway.Err(err))
		return nil
	}
	return seed
}

// Phase 1: Validate input and extract options

// Phase 2: Create and persist the initial TeamRun record

// Arm the run-level cumulative input-token budget gate (2026-08-24).
// Member rows accumulate into it in recordMemberUsage; on exceed the run
// ctx is cancelled so no further member steps are scheduled.

// Arm the run-level no-progress auditor (79-runtime-governance R5).
// Member step terminal statuses are fingerprinted at the same accounting
// point; correction note at N repeats, single-fire Cancel after M more.

// M82 P2-2: the sandbox per-run creation budget rides the same lifecycle —
// the cumulative counter is dropped when the run ends (live instances are
// untouched; they still die by Release/TTL/idle).

// Phase 3: Setup tracing and event emitter

// Phase 4: Resolve anchor agent and attachments

// Intent Pass overlaps compileTeamRuntime (chat C2). 2.5s fuse; DirectReply skips.

// Phase 5: Build TRPC dependencies and compile team runtime

// G2（ADR-F D2）：replanner 计数随 run 收口释放（含 HITL 暂停路径——
// 暂停即事件流结束；resume 重跑时重新注入回调并重新计数，与 graph run
// 域 Resume 语义对齐）。防 ManagedMap entry 泄漏（A5）。

// P0-03 fix: use manager agent ID as AppName for memory scope.
// Member agents share the manager's Runner session; their proactive
// recall uses their own agentID. Full per-member scope isolation
// requires per-member Runners (future work).

// defer 阶段请求 ctx 可能已取消：脱离取消语义（保留 trace 值）但必须有界。

// Phase 6: Build execution context with deadline, knowledge tools, and artifact collector

// M82 P2-2: tag the run ctx so sandbox acquisitions by team members
// (sandbox_fs / codeexecutor via SessionLeases) count against this run's
// cumulative creation budget.

// 79-runtime-governance（2026-08-27 二轮审查 H5 根修）：成员闸事件 run
// 归属通道——成员子 invocation 经框架 Clone 获新 uuid 后无法从
// InvocationID 回溯 run.ID，经 ctx 值注入供 param_rule_gate /
// loop_guard / tool_result_prune / context_compression 钩子取回
// （decision.GateRunIDFromContext → gateRunID），RunGateStats 聚合
// 才可命中；loop guard 隔离键同取此值恢复跨执行累计语义。

// T5（2026-08-27）：会话归属同坐标注入——成员闸钩子经
// decision.GateSessionIDFromContext 取回，SessionGateStats 聚合命中。

// P6-N3：成员模型调用链注入首字节静默重试预算（每次 GenerateContent
// 静默超 budget 自动重连）；下方消费端守卫总时限按全部重连尝试放宽，
// 避免守卫在重连中途取消 runCtx 误杀恢复。
// 2026-09-01 活性守卫治理：同点注入流中段 stall 预算（事件间隔静默
// 上限 + 重连次数上限），模型目录 config_json 可覆盖包默认值。

// Phase 6.5: Build project metadata and pre-configure the v2 ActivityProjector.
// N-21/N-03: The v2 projector is pre-configured and injected into runCtx so
// that plugins (cost_guard, model_router) and hooks (tool_confirmation) can
// emit notice/confirm events via biz.ActivityEmitterFromContext during the
// LLM call. Reset clears stale state; Configure sets meta without emitting
// events. OnTurnStart (called later by the stream consumer) will emit
// task.created + turn.started.

// M80 方案A 中流预算闸（2026-08-26 设计，08-27 深度审查补接线）：graph
// runtime 下 finish-path member 行全带 attribution 镜像标记，
// recordMemberUsage 的 genuine 累计分支（attribution==""）永不触发——
// 本钩子是 graph 路径 run 预算闸的唯一累计源。usage delta（跨成员逐轮
// 计费口径，accumulateStreamUsage 的 result.PromptTok 增量）同步累计，
// 首次超闸走 tripRunTokenBudget 四件套并 abortRun 本地中止在飞流——
// RunTeamTest 等旁路无 RunRegistry 活动条目，Runs.Cancel 空转，必须
// 本地取消。失败路径 partial genuine 行的 finish 累计是同向叠加
// （single-fire 防重跳），无双计风险。

// P2-1（2026-09-03 持久化心跳）：流式事件 → 节流写 team_runs_v2.heartbeat_at，
// biz idle 探测取 max(steps.started_at, heartbeat_at)——成员单 step 长流式
// 生成（如 32k 报告）不再被误判 idle 击杀。graph runtime 下并行成员共享
// 本 streamOpts，pinger 内部并发安全。

// Publish session activities for ALL members at the start of execution,
// regardless of graph/non-graph mode. Without these, the frontend
// AgentCard has no parent node to render, and member thinking/action/reply
// activities become orphaned at the root level.
//
// In graph mode, PublishTeamStepStarted may also fire later for each node
// start — the deterministic activity ID (SessionActivityID) ensures the
// frontend deduplicates and treats subsequent publishes as status updates
// rather than new AgentCards. Members that never execute stay in "running"
// until FinalizeGraphTeamRun or timeout transitions them.

// 批次 B：以锚点 agent 的 skill runtime policy 安装概览预算渲染器——
// graph runtime 会把 root RunOptions 传播到成员 invocation，一次安装全链生效。

// R6 fork id 空间统一（79-runtime-governance，路线A 框架补丁 ADR
// 2026-08-27）：team 会话 v2 turns_v2.id = run.ID
// （buildTeamProjectMeta InvocationID），框架根 invocation id 同源
// 对齐后 session_fork_repo 的 event->>'invocationId' 边界匹配才命中。
// 成员子 invocation 经 Clone 仍获新 uuid，边界语义不变。

// 2026-08-08 问题3c：DAG 下游团队把上游已完成团队的交付物种子注入
// graph 初始 state（deliverable StateField，MergeReducer）。graph runtime
// 会把 root RuntimeState 合并进 initialState（graph/trpc/builder.go），
// 成员节点的 node-start RuntimeState 快照随之携带种子——get_deliverable
// 直接读到上游 topic，不再因 per-execution state 隔离而 found=false。
// 种子回流不冒充本团队产出：biz 层闸门/信封写入用同一解析结果减种子。

// P2-1 模型级联 / P3-4 评测态 profile：definition 配置 model_cascade 时安装
// run 级 ModelSelector——leader（synthesizer/意图锚点）保持高档 base，其余
// 成员路由到成本档；配置 eval_profile 时钉住全成员模型/工具/生成参数
// （ADR-E：两者互斥，eval_profile 胜出——可复现性优先于成本优化）。
// agent-node 克隆父 RunOptions，成员 invocation 以各自 AgentName 应用
// selector；run 级 selector 优先于成员构建级 selector（团队显式管理策略，
// 覆盖成员自身的模型路由插件）。

// G2（ADR-F D2）：team 图执行注入 replanner 全局回调——节点静态声明
// （fallback_agent/on_failure=skip）未恢复的失败经 RuntimeReplanner 决策
// 落地（Reflexion 重试 / reroute→skip / insert_fallback→HITL）。非 graph
// 模式或未配置 replanner 时返回 nil（append nil option 无效，须条件追加）。

// F-C: 团队 Graph 路径绕过 trpcGraphRuntime.Run（唯一 EventBridge 宿主），
// node_start/node_end 到不了 coordinator 的 step watch，per-member
// team_run_steps 不持久化。tee 框架事件流，把图节点生命周期以 graph_stage
// system notice 重发到 EventBus（watch 按 spiritSessionID + execution_id 过滤）。
// Y3: HITL interrupt 经 tee 内联同步标记（先于流结束），避免 async bus
// 握手与 run 收尾竞态（DeferTeamRunSuccessIfHITL 读到陈旧 Running 把暂停误判完成）。

// HITL 标记可能触发于 run ctx 取消后：WithoutCancel 保留值、WithTimeout 兜底有界。

// Phase 7: Consume stream (streamOpts already has the pre-created projector)

// Context patching uses the final round's tokens (window occupancy),
// not the billing totals summed across rounds.

// P2.5：终止来源分列——RunRegistry.Cancel 记录 reason 到状态条目，
// 这里读回把 no_progress / team_token_budget_exceeded /
// user_cancel 等写进 run 终态记录（此前一律 client_disconnect_or_abort，
// 审计无法区分守卫终止与用户停止）。

// Phase 8: Build assistant message from stream result

// Phase 9: Validate output and persist assistant message

// Phase 10: Handle HITL defer, finalize run, and patch context usage

// （2026-09-01 删除 R1-a MemberUsage 补加块：accumulateStreamUsage 的
// billing 口径 result.PromptTok = ΣMemberUsage + ΣrootUsage 已含成员
// 用量，再补加导致 team_turn/run.token_in/spirit 镜像全链精确 2x——
// eval0831-s06-fix3 实证。R1-a 假设的「root=0 且成员有耗」场景在该口径
// 下不可能发生，块属多余。）

// P2-1b (2026-08-19): persist genuine per-member usage rows from the
// stream's MemberUsage — graph watch writes member steps with zero tokens,
// so without this pass a watch-healthy team run is invisible to billable
// aggregates (team_turn rows are excluded by design). When rows were
// written, the anchor-fallback usage row is suppressed (double-count guard).

// Persist swarm active member for CrossRequestTransfer (next turn entry override).

// Context patching uses the final round's tokens (window occupancy), not
// the billing totals; fall back to turn totals when usage was estimated.
