package agent

import (
	"context"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/tools/skillruntime"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// buildCallbackChainOptions wires product-layer Callback Chain into LLMAgent.
// Runner-level plugins (WithPlugins) handle DB builtins and OnEvent; see plugintrpc/orchestration.go.
// gate is the per-build shared confirmation gate (nil when nothing is gated).
// skillRepo/skillFilter feed the skill_overview budget hook (F5); nil when the
// agent has no skills configured.
func buildCallbackChainOptions(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, gate *toolConfirmGate, catalog *toolBuildCatalog, skillRepo trpcskill.Repository, skillFilter trpcskill.VisibilityFilter) ([]trpcllmagent.Option, *biztool.CircuitBreakerRegistry) {
	chain, cbRegistry := productCallbackChainWithRegistry(ctx, ag, deps, gate, catalog, skillRepo, skillFilter)
	if chain == nil {
		return nil, nil
	}
	if deps.PluginManager != nil {
		chain = deps.PluginManager.MergeChain(ctx, ag.ID, ag.AgentKey, chain)
	}
	var opts []trpcllmagent.Option
	if chain.HasAgentHooks() {
		opts = append(opts, trpcllmagent.WithAgentCallbacks(chain.AdaptAgentCallbacks()))
	}
	if chain.HasModelHooks() {
		opts = append(opts, trpcllmagent.WithModelCallbacks(chain.AdaptModelCallbacks()))
	}
	if chain.HasToolHooks() {
		opts = append(opts, trpcllmagent.WithToolCallbacks(chain.AdaptToolCallbacks()))
	}
	return opts, cbRegistry
}

func productCallbackChain(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, gate *toolConfirmGate) *callbacks.Chain {
	chain, _ := productCallbackChainWithRegistry(ctx, ag, deps, gate, nil, nil, nil)
	return chain
}

// catalog is the per-build tool snapshot (PERF-1) feeding the result-cache
// hooks; nil disables result caching for the build (fail-soft).
func productCallbackChainWithRegistry(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, gate *toolConfirmGate, catalog *toolBuildCatalog, skillRepo trpcskill.Repository, skillFilter trpcskill.VisibilityFilter) (*callbacks.Chain, *biztool.CircuitBreakerRegistry) {
	var entries []callbacks.Callback
	lg := deps.Logger()
	entries = append(entries, productChainLifecycleMetrics()...)

	if hook := newStaticRuntimeCueBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newDynamicRuntimeCueBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	// 静态目录 cue（WP-4）：deferred 工具的「名称+描述」清单，追加到消息末尾。
	if hook := newToolCatalogCueBeforeHook(deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newContextCompressionBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newSkillGuidanceBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newWorkspaceSkillsBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newMemoryInjectBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	// 包A（session-eval-20260825 A1）：管理层装配预算硬闸。priority 8 = 全量
	// 注入（memory 5 / knowledge 6 / 各 cue ≤6）之后、终审压缩闸（9）与 L0
	// 快照（10）之前，计量口径=完全注入后的请求。hard<=0（默认）时 hook 为
	// nil——轻链路零开销，管理层经 SQL 灰度开启（assembly_budget_* 列）。
	if hook := newAssemblyBudgetBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newWorkingMemoryContextBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newMemoryEditContextBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newCompactContextBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newToolResultGateBeforeHook(deps.ToolResultGate, ag, lg); hook != nil {
		entries = append(entries, hook)
	}
	// R2 确定性工具结果剪枝（79-runtime-governance）：priority 7 = 全量注入类
	// hook（≤6）之后、装配预算（8）与终审压缩（9）之前——剪枝削峰 → 阈值压缩
	// 兜底。gate 为 nil 或 runtime.tool_result_prune.enabled=false 时不注册。
	// R7（G-1）：实际剪枝双写 system_guard 决策记录，供 run 统计聚合。
	if hook := newToolResultPruneBeforeHook(deps.ToolResultGate, deps.ToolResultPrune, lg, deps.DecisionCollector); hook != nil {
		entries = append(entries, hook)
	}
	// Team completion guard: prevent Spirit LLM from polling get_team_deliverable
	// when teams are still running. Enforces system-push pattern over LLM-polling.
	if hook := newTeamCompletionGuardBeforeHook(deps.TeamCompletionChecker, lg); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newFactQueryWebGuardBeforeHook(deps.WebResearchReady); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newOrchestrationPhasePromoteBeforeHook(deps.DeferredManager, lg); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newIntentToolHintPromoteBeforeHook(deps.DeferredManager, lg); hook != nil {
		entries = append(entries, hook)
	}
	entries = append(entries, newOrchestrationBriefBeforeHook())
	if hook := newKnowledgeCueBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	// 上下文预算台账（29-token §9.6）：tools_schema + history + static_prefix 计量。无条件注册——
	// ctx 无收集器时每次仅一次 ctx 读，与缓存 BUILD 产物共享安全。
	entries = append(entries, newContextBudgetToolsBeforeHook())
	// N3：history 计量（最终请求中的非 system 消息）。
	entries = append(entries, newContextBudgetHistoryBeforeHook())
	// Instruction / 系统前缀计量（IDENTITY 等），与 runtime cue 的 285 字区别开。
	entries = append(entries, newContextBudgetStaticPrefixBeforeHook())
	// F5：skill_overview 计量（镜像框架 overview 渲染，零额外 DB 查询）。
	// 批次 B 计量对齐：预算与 RunOptionWithOverviewBudget 安装的渲染器同值，
	// 计量截断后的实际注入文本。typed-nil 防护同 trpc_build.go runtimeIface。
	var skillRuntime skillruntime.RuntimeSettings
	if ag.Settings != nil {
		skillRuntime = ag.Settings
	}
	if hook := newContextBudgetSkillOverviewBeforeHook(skillRepo, skillFilter, skillruntime.OverviewBudgetFromRuntime(skillRuntime)); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newPromptSnapshotBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	// Voice Fast-Path：语音轮次 per-request 关思考（ctx 标记驱动，与入口无关的
	// 缓存 BUILD 产物共享安全）。无条件注册——非语音轮次仅一次 ctx 读。
	entries = append(entries, newVoiceFastPathBeforeHook())
	// Problem 6: inject a reply reminder after each tool call so the LLM
	// outputs a brief "已完成 + 下一步" reply before calling the next tool.
	// BeforeModel hook reads state set by the AfterTool hook.
	// S2 (2026-08-18): gated by ReplyReminderEnabled — evaluation/single-tool
	// agents can disable it to skip the extra LLM summary call (~3.5s).
	if ag.Settings != nil && ag.Settings.ReplyReminderEnabled {
		entries = append(entries, newReplyReminderBeforeHook())
	}
	// P2 TTFT: the framework content processor appends intent context right
	// after the system block (before session history), which invalidates the
	// prompt-cache prefix every turn. This hook (priority 100, runs after all
	// other message-mutating hooks) moves it to the END of the message list.
	entries = append(entries, newIntentReorderBeforeHook())
	if hook := newL0SnapshotAfterModelHook(deps); hook != nil {
		entries = append(entries, hook)
	}
	entries = append(entries, newTokenUsageAccumulatorAfterHook())

	var cbRegistry *biztool.CircuitBreakerRegistry
	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		// Tool execution timeout: inject BeforeTool + AfterTool hooks that
		// enforce a per-tool timeout via context.WithTimeout. This is the
		// product-layer implementation since the framework lacks built-in timeout.
		// P1-2：timeout 每调用经 policyResolver 查询（策略变更零重建生效），
		// resolver miss 回退构建期快照（ag.Settings.ToolsExecutionTimeoutSec）。
		policyAgentID := ag.ID
		buildTimeoutSec := ag.Settings.ToolsExecutionTimeoutSec
		if timeoutHooks := toolExecutionTimeoutHooks(func() time.Duration {
			return toolExecutionTimeoutFor(policyAgentID, buildTimeoutSec)
		}, lg); len(timeoutHooks) > 0 {
			entries = append(entries, timeoutHooks...)
		}
		entries = append(entries, newToolArgsRepairBeforeHook(lg))
		entries = append(entries, newTodoArgsGuardBeforeHook(lg))
		// 79-runtime-governance R9 参数门禁（P5.4）：priority 3 与 args 守卫同档，
		// 链为稳定排序（同档先追加先执行），故追加在 args 守卫之前——deny 拦截点
		// 早于 args 守卫/循环守卫（4）/确认门禁（10）；args repair（1）之后求值，
		// 匹配的是修复后的真实参数文本。ToolUC 未装配时 hook 为 nil 不注册。
		if hook := newParamRuleGateBeforeHook(ag, deps); hook != nil {
			entries = append(entries, hook)
		}
		entries = append(entries, newToolArgsGuardBeforeHook(lg))
		// tool-not-found 纠错反馈：错误回执附当前可用工具清单（req.Tools 按
		// invocation 实际工具面填充），防止模型臆造变体名重试空转。
		entries = append(entries, newToolNotFoundFeedbackBeforeHook(lg))
		// 工具循环守卫：同工具+同参数+同结果的连续成功调用判定为无效空转，
		// 第 3 次起以 CustomResult 纠偏拦截（priority 4，先于熔断器/确认门禁）。
		// modelHook（A2'b 空转轮次早停）：BeforeModel 结算上轮工具产出，
		// 连续零产出轮注入降级引导；满阈值的工具面封锁由 beforeHook 执行。
		loopGuard := newToolLoopGuard(lg)
	// M80：loop_guard_blocked 决策双写（设计 §3.2 row 3）。
	loopGuard.setDecisionCollector(deps.DecisionCollector)
	// 79-runtime-governance 二轮 Q1（S02「合法失控」根修）：行为模式闸阈值
	// 每调用经 policyResolver 读取——注入 agentID（resolver 键）+ 构建期快照
	// （resolver miss 兜底），策略变更零重建生效。
	loopGuard.setGateThresholds(ag.ID, ag.Settings.LoopGuardToolLoadMax, ag.Settings.LoopGuardWallSoftSec, ag.Settings.LoopGuardWallHardSec)
	entries = append(entries, loopGuard.beforeHook(), loopGuard.afterHook(), loopGuard.modelHook())
		entries = append(entries, newToolResultCacheBeforeHook(deps, catalog))
		entries = append(entries, newToolCallTimingBeforeHook())
		entries = append(entries, newWorkspaceSandboxBeforeHook(ag, deps))
		entries = append(entries, newEditDisciplineBeforeHook(ag, deps))
		if hook := newMCPCatalogRefreshAfterHook(deps.MCPCacheInvalidators, lg); hook != nil {
			entries = append(entries, hook)
		}
		entries = append(entries, newShellOnFailureAfterHook())
		if gate != nil {
			entries = append(entries, newToolConfirmationBeforeHook(gate, ag, deps))
		}
		// Capture skill_load/skill_run slug into invocation state BEFORE the
		// tool recorder reads it (recorder runs at priority 50).
		entries = append(entries, newSkillLoadCaptureAfterHook())
		// Problem 6: set reply-reminder state after each tool call so the
		// BeforeModel hook can inject a reminder system message.
		// S2 (2026-08-18): gated by ReplyReminderEnabled.
		if ag.Settings != nil && ag.Settings.ReplyReminderEnabled {
			entries = append(entries, newReplyReminderAfterHook())
		}
		entries = append(entries, callbacks.NewToolRecorderCallback(50, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
			recordToolInvocationAfter(ctx, args, ag, deps)
			return &trpctool.AfterToolResult{}, nil
		}))
		entries = append(entries, newToolResultCacheAfterHook(deps, catalog))
		// P0-1 终态补偿跟踪：声明了逆工具的正向工具（如 gns3_fault_inject）调用
		// 成功后记 pending，逆工具调用成功核销；超时未核销产出
		// ops.compensation_pending 告警。进程级单例，agent 重建不丢状态。
		entries = append(entries, compensationTrackerAfterHook(lg))
		// Side-effect feedback: remind the LLM (via tool results) when files
		// were modified without a subsequent test run. The BeforeAgent hook
		// pre-creates a per-invocation ToolReminder so concurrent sessions
		// sharing a cached Agent never share reminder state.
		entries = append(entries, newToolReminderBeforeAgentHook())
		entries = append(entries, newToolReminderAfterHook())
		if ag.Settings.ToolsCircuitBreakerEnabled {
			cbRegistry = buildCircuitBreakerRegistry(ag.Settings, lg)
			entries = append(entries, newCircuitBreakerBeforeHook(cbRegistry, lg))
			entries = append(entries, newCircuitBreakerAfterHook(cbRegistry, lg))
		}
		if ag.Settings.ToolsCommandSafetyEnabled {
			entries = append(entries, newCommandSafetyBeforeHook(lg))
		}
		// Output size limiter: fallback truncation for undecorated string
		// results. Decorator envelopes / budget-override tools are skipped.
		// Runs after the tool recorder (priority 50) so original size is logged.
		entries = append(entries, newOutputSizeLimiterAfterHook(lg))
	}

	if len(entries) == 0 {
		return nil, nil
	}
	return callbacks.NewChain(entries...), cbRegistry
}
