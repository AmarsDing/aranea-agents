package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/agent/a2ui"
	localexec "aranea-agents/internal/agent/codeexecutor"
	agentplanner "aranea-agents/internal/agent/planner"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/skill/storage"
	skilltrpc "aranea-agents/internal/skill/trpc"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/extension/toolpipe"
	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctiktoken "trpc.group/trpc-go/trpc-agent-go/model/tiktoken"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// generationConfigForAgent 构建主运行时 GenerationConfig（P2-5）：Stream 恒开；
// agent 静态推理策略（reasoning_mode=custom 时的 reasoning_level）映射为思考
// 强度参数——off→ThinkingEnabled=false、low/medium/high/max→ReasoningEffort。
// 默认（provider_default/未配置/非法档）不注入任何 thinking 字段，保留 provider
// 服务端默认——存量 agent 零行为变化。单次调用的复杂度覆盖走旁路 caller 的
// LLMCallRequest.ThinkingEffort（biz.ResolveThinkingEffort），与此静态档分层。
func generationConfigForAgent(ag biz.Agent) trpcmodel.GenerationConfig {
	cfg := trpcmodel.GenerationConfig{Stream: true}
	if ag.Settings == nil {
		return cfg
	}
	rc := ag.Settings.GetReasoning()
	switch eff := biz.StaticThinkingEffort(rc.Mode, rc.Level); eff {
	case "":
		// 跟随厂商：不注入。
	case biz.ThinkingEffortOff:
		disabled := false
		cfg.ThinkingEnabled = &disabled
	default:
		cfg.ReasoningEffort = &eff
	}
	return cfg
}

func BuildTRPCLLMAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcagent.Agent, error) {
	a, _, _, err := buildTRPCLLMAgentWithToolSets(ctx, ag, deps, lg)
	return a, err
}

// buildTRPCLLMAgentWithToolSets builds the agent and also returns the
// ToolSets created during assembly. The caller (cache layer) uses these
// ToolSets to close them when the agent is evicted, preventing resource
// leaks (MCP sessions, stdio subprocesses, HTTP connections).
// 第四返回值为 P0-2B 热替换元数据（faceMeta），随缓存 entry 持久化；
// 无工具计划（sp==nil）时返回 nil face，该 entry 不参与换面。
func buildTRPCLLMAgentWithToolSets(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcagent.Agent, []trpctool.ToolSet, *faceMeta, error) {
	if strings.TrimSpace(ag.AgentKey) == "" {
		return nil, nil, nil, apierror.BadRequest(apierror.DomainAgent, "agent_key required")
	}
	// Always use the Agent's own provider/model for the cached build.
	// Per-request provider/model overrides are applied via RunOption
	// (agent.WithModel) at turn execution time, so they don't need to
	// be baked into the agent build — this is what enables cache key
	// simplification (removing Provider/Model from the fingerprint).
	//
	// When the agent has no provider/model configured (e.g. spirit agent
	// configured to inherit from the chat interface), we resolve a system
	// default model for the build. The actual model used at runtime will
	// be overridden by WithModel RunOption from the chat request.
	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	agentModelEmpty := prov == "" || mod == ""
	if agentModelEmpty {
		prov, mod = resolveBuildDefaultModel(ctx, deps, lg)
		if prov == "" || mod == "" {
			return nil, nil, nil, apierror.BadRequest(apierror.DomainAgent, "agent provider and model required (no system default available)")
		}
		lg.Info("Agent 无配置模型，使用系统默认模型构建",
			loggateway.StepID("agent.build_default_model"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Str("agent_key", ag.AgentKey),
			loggateway.Str("provider", prov),
			loggateway.Str("model", mod))
	}

	lg.Info("Agent 构建", loggateway.StepID("agent.build"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("provider", prov), loggateway.Str("model", mod))

	// K6 子步骤耗时测量：冷构建实测可达数秒，汇总日志拆解 model/prompt/
	// skill/tools/finalize/new 各段耗时，超预算定位用（仅在 cache miss 构建时发射）。
	buildStart := time.Now()
	var modelMs, promptMs, skillMs, toolsMs int64

	modelStart := time.Now()
	m, err := provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, prov, mod, lg)
	if err != nil {
		// Fallback: when the agent's configured model is not found in the catalog
		// (e.g. model was disabled or deleted after agent creation), fall back to
		// the system default model. The actual runtime model will be overridden by
		// WithModel RunOption from the chat request, so the build-time model only
		// needs to be valid for agent construction.
		if !agentModelEmpty && errors.Is(err, biz.ErrProviderModelNotFound) {
			lg.Warn("Agent 配置模型在目录中不可用，回退到系统默认模型",
				loggateway.StepID("agent.build_model_fallback"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Str("agent_key", ag.AgentKey),
				loggateway.Str("orig_provider", prov),
				loggateway.Str("orig_model", mod),
				loggateway.Err(err))
			fbProv, fbMod := resolveBuildDefaultModel(ctx, deps, lg)
			if fbProv != "" && fbMod != "" {
				prov, mod = fbProv, fbMod
				m, err = provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, prov, mod, lg)
			}
		}
		if err != nil {
			lg.Error("Agent 构建失败：模型解析", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Str("provider", prov), loggateway.Str("model", mod), loggateway.Err(err))
			return nil, nil, nil, err
		}
	}
	modelMs = time.Since(modelStart).Milliseconds()

	files := ag.Files
	if len(files) == 0 && deps.Agents != nil {
		promptStart := time.Now()
		files, err = deps.Agents.ListAgentPromptFiles(ctx, ag.ID)
		if err != nil {
			lg.Error("Agent 构建失败：提示文件加载", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
			return nil, nil, nil, err
		}
		promptMs = time.Since(promptStart).Milliseconds()
	}
	// PGO-1-AGENT-02: Inject 岗位职责 from the category tree when the flag is on
	// and the agent has a position associated (PositionID != "").
	var catResp string
	if shouldInjectCategoryResponsibility(ag) && deps.Organization != nil {
		if resp, err := deps.Organization.BuildResponsibility(ctx, ag.PositionID, ag.SystemPromptMode); err != nil {
			lg.Warn("岗位职责注入失败", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
		} else {
			catResp = resp
		}
	}
	if indCtx := BuildIndustryContext(ctx, Deps{
		Agents: deps.Agents, AgentUC: deps.AgentUC,
		Organization: deps.Organization, LG: deps.Logger(),
	}, ag); indCtx != "" {
		if catResp != "" {
			catResp += "\n\n" + indCtx
		} else {
			catResp = indCtx
		}
	}
	sys := BuildSystemPrompt(ag, files, ag.SystemPromptMode, catResp)
	// RuntimeCapabilityCue is injected per LLM call via BeforeModel (runtime_cue_inject.go).
	opts := []trpcllmagent.Option{
		trpcllmagent.WithModel(m),
	}
	hasPluginModelRouter := false
	hasPluginCostGuard := false
	var modelSelectors []trpcagent.ModelSelector
	if deps.PluginManager != nil {
		// N-B1：插件配置按调用方工作区过滤，杜绝跨租户配置泄漏。
		pluginWsID := workspace.IDFromContext(ctx)
		if routerCfg, ok := deps.PluginManager.ModelRouterConfigForAgent(ag.ID, pluginWsID); ok {
			hasPluginModelRouter = true
			modelSelectors = append(modelSelectors,
				PluginModelSelector(prov, mod, deps.ModelCatalog, deps.RT, routerCfg, lg),
			)
		}
		if cgCfg, ok := deps.PluginManager.CostGuardConfigForAgent(ag.ID, pluginWsID); ok {
			hasPluginCostGuard = true
			modelSelectors = append(modelSelectors,
				// N-B2：tracker 按请求 ctx 解析，与运行时 cost_guard 插件共用同一预算桶。
				PluginCostGuardSelector(prov, mod, deps.ModelCatalog, deps.RT, cgCfg, deps.PluginManager.BudgetTrackerForContext, lg),
			)
		}
	}
	if len(modelSelectors) > 0 {
		opts = append(opts, trpcllmagent.WithModelSelector(ChainedModelSelector(modelSelectors...)))
	}
	opts = append(opts,
		trpcllmagent.WithInstruction(sys),
		trpcllmagent.WithDescription(IdentityDescriptionForAgent(ag, files)),
		trpcllmagent.WithChannelBufferSize(256),
		trpcllmagent.WithGenerationConfig(generationConfigForAgent(ag)),
	)

	var pipeline *a2ui.Pipeline
	if plannerKind(ag) == "a2ui" {
		pipeline = a2ui.NewPipeline(lg)
	}
	if p := agentplanner.Select(deps.DialogMode, plannerKind(ag), plannerConfigJSON(ag), pipeline); p != nil {
		opts = append(opts, trpcllmagent.WithPlanner(p))
	}

	// Hoisted for the skill_overview budget hook (F5): the callback chain is
	// built later and needs the same repo/filter the framework processor uses.
	var skillRepoForBudget trpcskill.Repository
	var skillFilterForBudget trpcskill.VisibilityFilter
	if deps.SkillUC != nil {
		skillStart := time.Now()
		repo, filter, exec, err := buildSkillDeps(ctx, ag, deps)
		if err != nil {
			lg.Error("Agent 构建失败：技能依赖", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
			return nil, nil, nil, err
		}
		skillRepoForBudget, skillFilterForBudget = repo, filter
		skillMs = time.Since(skillStart).Milliseconds()
		if repo != nil {
			opts = append(opts, trpcllmagent.WithSkills(repo))
		}
		if filter != nil {
			opts = append(opts, trpcllmagent.WithSkillFilter(filter))
		}
		if exec != nil {
			opts = append(opts, trpcllmagent.WithCodeExecutor(exec))
		}
		skillProfile, dirHints := skillOptionsForPromptMode(ag.SystemPromptMode)
		if ag.Settings != nil && biz.IsProgressiveSkillLoad(ag.Settings.GetSkillLoadMode()) {
			// Progressive mode: enable tool result mode and directory hints.
			// skillProfile is preserved as-is from skillOptionsForPromptMode:
			//   - "complete" mode → Full profile (includes skill_run)
			//   - other modes → KnowledgeOnly profile (skill_load/list_docs/select_docs only)
			dirHints = true
			opts = append(opts,
				trpcllmagent.WithSkillsLoadedContentInToolResults(true),
			)
		}
		opts = append(opts,
			trpcllmagent.WithSkillToolProfile(skillProfile),
			trpcllmagent.WithSkillsDirectoryHints(dirHints),
		)
	}

	var assembledToolSets []trpctool.ToolSet
	toolsStart := time.Now()
	// 批量预载（P0 性能修复）：eff 键集 + 工具构建目录快照 + 确认门各加载一次，
	// 在 buildToolsetsForAgent 与 buildCallbackChainOptions 之间共享，替代原先
	// 三处各自 N 次的 GetTool 聚合 SQL（冷构建 70 工具 × 3 ≈ 210 次、约 10s）。
	var eff map[string]bool
	var catalog *toolBuildCatalog
	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		eff = loadEffectiveToolKeys(ctx, deps, ag.ID)
		catalog = loadToolBuildCatalog(ctx, ag.ID, eff, deps)
	}
	// Gate is built even when ToolsEnabled=false so plugin confirmation guards
	// still cover custom/kanban/memory tools (previous behavior).
	gate := buildToolConfirmGate(ctx, ag, deps, catalog.confirmCatalog(eff))
	plan := &toolBuildPlan{eff: eff, catalog: catalog, gate: gate}
	var face *faceMeta
	if ts, retireUnits, sp, err := buildToolsetsForAgent(ctx, ag, deps, plan); err != nil {
		lg.Error("Agent 构建失败：工具构建", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
		return nil, nil, nil, apierror.Internal(apierror.DomainAgent, "tool build failed").WithCause(err)
	} else if ts != nil {
		toolsMs = time.Since(toolsStart).Milliseconds()
		if len(ts.ToolSets) > 0 {
			opts = append(opts, trpcllmagent.WithToolSets(ts.ToolSets))
			// WithRefreshToolSetsOnRun is intentionally set to false (disabled).
			// Previously this was true, causing 0.2-5s MCP Initialize+ListTools
			// on every LLM call. Now MCP ToolSets are initialized once during
			// Agent build and cached. When MCP servers change, the agent cache
			// is invalidated (via MCPVersionHash change) and a fresh agent is
			// built with the updated tool list.
		}
		// P0-2 阶段A：entry 持有的是分片引用占位符（retire 单元），而非共享
		// 分片产物本体；entry 换代/驱逐时 graveyard Close 占位符 = 释放分片
		// 引用，产物由 shardCache 在 refs==0 且 LRU 淘汰时关闭。
		assembledToolSets = retireUnits
		// P0-2B：面元数据随 entry 持久化，miss 路径凭此做热替换兄弟匹配
		// 与四道门禁。指纹配方与 BuildTRPCLLMAgentCached 的 key 计算一致。
		face = faceMetaFromPlan(computeBuildKeyFP(ag, deps, deps.ToolVersionHash, deps.SkillVersionHash, deps.MCPVersionHash), sp, ts)
		if len(ts.Tools) > 0 {
			opts = append(opts, trpcllmagent.WithTools(ts.Tools))
		}
		if ts.DeferredManager != nil {
			deps = deps.WithDeferredManager(ts.DeferredManager)
		}
		// Enable TodoEnforcer only when the agent already has the todo_write
		// tool enabled. The enforcer contributes todo_declare_blocker + enforcement
		// callbacks (BeforeModel/AfterModel) that prevent the agent from declaring
		// "done" while open todo items remain. The enforcer's own todo_write tool
		// is silently dropped by LLMAgent's earlier-wins dedup (user tool wins),
		// but enforcement still works because both tools write to the same session
		// state key (temp:todos:<branch>).
		if hasToolByName(ts.Tools, "todo_write") {
			opts = append(opts, NewTodoEnforcerOption(nil, lg))
		}
	}

	finalizeStart := time.Now()
	if biz.ResolveMemoryRuntimePolicy(ag.Settings).MasterEnabled {
		if !deps.HasMemory {
			lg.Warn("Agent 已启用记忆但未配置 MemoryService，记忆工具已禁用",
				loggateway.StepID("agent.memory_disabled"),
				loggateway.Str("agent_id", ag.ID))
		}
	}

	cbStart := time.Now()
	if chainOpts, cbRegistry := buildCallbackChainOptions(ctx, ag, deps, gate, catalog, skillRepoForBudget, skillFilterForBudget); len(chainOpts) > 0 {
		opts = append(opts, chainOpts...)
		if cbRegistry != nil {
			deps = deps.WithCircuitBreakerRegistry(cbRegistry)
		}
	}
	cbMs := time.Since(cbStart).Milliseconds()

	settingsStart := time.Now()
	if ag.Settings != nil {
		opts = append(opts, buildTRPCRuntimeOptions(ag.Settings, hasPluginModelRouter || hasPluginCostGuard, prov, mod, deps.ModelCatalog, deps.RT, lg)...)
		opts = append(opts, SafetyLimitAdapter(ag, lg)...)
		opts = append(opts, KnowledgeAdapter(ctx, ag, deps, lg)...)

		if toolFilter := buildToolFilter(ag.Settings, deps.DeferredManager, lg); toolFilter != nil {
			opts = append(opts, trpcllmagent.WithToolFilter(toolFilter))
		}

		if retryPolicy := buildToolRetryPolicy(ag.Settings); retryPolicy != nil {
			opts = append(opts, trpcllmagent.WithToolCallRetryPolicy(retryPolicy))
		}

		if ag.Settings.ToolsParallelEnabled {
			opts = append(opts, trpcllmagent.WithEnableParallelTools(true))
		}
	}
	settingsMs := time.Since(settingsStart).Milliseconds()

	// ToolPipe Extension: enables LLM to filter long tool results (grep/head/tail/jq),
	// reducing token consumption by 50-90% on large outputs. Uses WithToolScope to
	// dynamically match MCP tools and known long-output tools, avoiding interference
	// with framework-managed tools (todo, memory, skill_*).
	opts = append(opts, trpcllmagent.WithExtensions(
		toolpipe.New(
			toolpipe.WithToolScope(isToolPipeEligible),
			toolpipe.WithAllowedOps(toolpipe.OpGrep, toolpipe.OpHead, toolpipe.OpTail),
		),
	))

	finalizeMs := time.Since(finalizeStart).Milliseconds()
	newStart := time.Now()
	a := trpcllmagent.New(strings.TrimSpace(ag.AgentKey), opts...)
	newMs := time.Since(newStart).Milliseconds()

	emitBuildDoneSummary(lg, ag, buildStart, buildPhaseDurations{
		model: modelMs, prompt: promptMs, skill: skillMs, tools: toolsMs,
		cb: cbMs, settings: settingsMs, finalize: finalizeMs, newAgent: newMs,
	})

	return a, assembledToolSets, face, nil
}

// buildPhaseDurations carries the K6 per-phase build timings (milliseconds).
type buildPhaseDurations struct {
	model, prompt, skill, tools, cb, settings, finalize, newAgent int64
}

// emitBuildDoneSummary 发射构建完成汇总日志：各子步骤耗时拆解，超预算（>2s）升级 Warn
// 便于运维巡检。冷构建黑盒（实测 6-10s）按 model/prompt/skill/tools/finalize/new 六段
// 归因，超预算排查直接读本行，无需二次插桩。
func emitBuildDoneSummary(lg loggateway.Logger, ag biz.Agent, buildStart time.Time, d buildPhaseDurations) {
	totalMs := time.Since(buildStart).Milliseconds()
	fields := []loggateway.Field{
		loggateway.StepID("agent.build_done"),
		loggateway.Str("agent_id", ag.ID),
		loggateway.Str("agent_key", ag.AgentKey),
		loggateway.Int64("total_ms", totalMs),
		loggateway.Int64("model_ms", d.model),
		loggateway.Int64("prompt_ms", d.prompt),
		loggateway.Int64("skill_ms", d.skill),
		loggateway.Int64("tools_ms", d.tools),
		loggateway.Int64("finalize_ms", d.finalize),
		loggateway.Int64("new_ms", d.newAgent),
		loggateway.Int64("cb_ms", d.cb),
		loggateway.Int64("settings_ms", d.settings),
	}
	if totalMs > 2000 {
		lg.Warn("Agent 构建完成（超预算）", fields...)
	} else {
		lg.Info("Agent 构建完成", fields...)
	}
}

// buildSkillDeps resolves the Skill repository, per-invocation visibility filter, and code executor.
// EP-BIZ-01: when SkillDBRepo is injected, the DB repo is the primary backend;
// the local executor falls back to the FS root so skill code files can still be run.
// Layer A (allow/deny) visibility is applied via skillruntime.AgentVisibilityFilter
// from agent_runtime_settings.skill_runtime_json; Layer B per-turn routing is
// injected as dynamic guidance cues via skill_guidance_inject.go.
func buildSkillDeps(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (trpcskill.Repository, trpcskill.VisibilityFilter, codeexecutor.CodeExecutor, error) {
	lg := deps.Logger()
	slugs, err := deps.SkillUC.ListEnabledPublishedSkillKeys(ctx)
	if err != nil || len(slugs) == 0 {
		lg.Warn("技能构建：无可用技能",
			loggateway.StepID("agent.skill_build"),
			loggateway.Err(err),
			loggateway.Str("slug_count", fmt.Sprintf("%v", len(slugs))))
		return nil, nil, nil, err
	}
	lg.Info("技能构建：解析中",
		loggateway.StepID("agent.skill_build"),
		loggateway.Str("flow_status", "done"),
		loggateway.Str("slug_count", fmt.Sprintf("%v", len(slugs))))

	// Always resolve rootDir so the executor has a valid path regardless of
	// which repo backend is selected.
	rootDir := storage.ResolveRoot()
	if deps.Sys != nil {
		if st, e := deps.Sys.Get(ctx); e == nil {
			rootDir = storage.ResolveRootWithPlatform(st.RootDirectory)
		}
	}

	var repo trpcskill.Repository
	if deps.SkillDBRepo != nil {
		// EP-BIZ-01: DB backend is the default; FS is dev fallback when absent.
		repo = deps.SkillDBRepo
	} else {
		fsRepo, err := skilltrpc.NewFSRepositoryAdapter(rootDir)
		if err != nil {
			return nil, nil, nil, err
		}
		repo = fsRepo
	}

	var runtime *biz.AgentRuntimeSettings
	if ag.Settings != nil {
		runtime = ag.Settings
	}
	// Avoid typed-nil interface: passing nil *biz.AgentRuntimeSettings to a
	// RuntimeSettings parameter creates a non-nil interface wrapping a nil
	// pointer, causing panic on method calls (resolve.go opts.Runtime != nil).
	// Use a nil interface instead so the nil check in ResolveSkillSlugsDetailed
	// works correctly.
	var runtimeIface skillruntime.RuntimeSettings
	if runtime != nil {
		runtimeIface = runtime
	}
	// Layer A-only visibility filter: keeps the framework skill overview
	// (system prompt prefix) byte-stable across turns for prompt-cache hits.
	// Per-turn Layer B routing lives in the progressive guidance injection
	// path (skill_guidance_inject.go) and never hides skills from the overview.
	filter := skillruntime.NewAgentVisibilityFilter(runtimeIface)

	execType := ""
	if runtime != nil {
		execType = runtime.GetCodeExecutor().Type
	}
	factory := deps.CodeExecFactory
	if factory == nil {
		factory = localexec.NewFactoryWithLogger(deps.Logger())
	}
	exec := skilltrpc.NewExecutorForAgent(ctx, factory, execType, rootDir, deps.Logger())
	lg.Info("技能构建完成",
		loggateway.StepID("agent.skill_build"),
		loggateway.Str("flow_status", "done"),
		loggateway.Str("slug_count", fmt.Sprintf("%v", len(slugs))),
		loggateway.Str("repo_type", fmt.Sprintf("%T", repo)))
	return repo, filter, exec, nil
}

func buildTRPCRuntimeOptions(s *biz.AgentRuntimeSettings, skipRuntimeModelSelector bool, prov, mod string, catalog biz.TeamModelCatalog, rt *provider.RoundTrip, lg loggateway.Logger) []trpcllmagent.Option {
	var opts []trpcllmagent.Option

	if s.ModelInstructionsJSON != "" && s.ModelInstructionsJSON != "{}" {
		var instructions map[string]string
		if err := json.Unmarshal([]byte(s.ModelInstructionsJSON), &instructions); err == nil && len(instructions) > 0 {
			opts = append(opts, trpcllmagent.WithModelInstructions(instructions))
		}
	}

	if s.ContextCompactionEnabled {
		opts = append(opts, trpcllmagent.WithEnableContextCompaction(true))
		if s.L0SummaryThreshold > 0 {
			opts = append(opts, trpcllmagent.WithContextCompactionThresholdRatio(s.L0SummaryThreshold))
		}
		keepRecent := s.L0SummaryKeepTurns
		if keepRecent <= 0 && s.L0RecentWindowTurns > 0 {
			keepRecent = s.L0RecentWindowTurns
		}
		if keepRecent > 0 {
			opts = append(opts, trpcllmagent.WithContextCompactionKeepRecentRequests(keepRecent))
		}
		// Pass 2: truncate oversized tool results (including current request)
		// to prevent large command outputs from overflowing the LLM context window.
		// Uses the framework's recommended 8192-token threshold, which preserves
		// head+tail of the output so the model can see both structure and results.
		opts = append(opts, trpcllmagent.WithContextCompactionOversizedToolResultMaxTokens(8192))

		// Use tiktoken-based counter for precise token estimation when
		// context compaction is enabled. Falls back to SimpleTokenCounter
		// if tiktoken initialization fails (e.g., unsupported model name).
		if counter, err := trpctiktoken.New(mod); err == nil {
			opts = append(opts, trpcllmagent.WithContextCompactionTokenCounter(counter))
		} else {
			lg.Warn("tiktoken 初始化失败，使用默认估算器",
				loggateway.StepID("agent.tiktoken_fallback"),
				loggateway.Str("model", mod),
				loggateway.Err(err))
		}
	}

	if s.SessionSummaryEnabled {
		opts = append(opts, trpcllmagent.WithAddSessionSummary(true))
		// P0-B: inject the summary as a user message (tail append area) instead
		// of the framework default "system" mode, which merges the summary into
		// system[0]. The summary changes after every compression, so merging it
		// into the static prefix byte-breaks the provider prompt cache.
		opts = append(opts, trpcllmagent.WithSessionSummaryInjectionMode(trpcllmagent.SessionSummaryInjectionUser))
	}

	if s.MemoryEnabled && s.MemoryMaxResults > 0 {
		opts = append(opts, trpcllmagent.WithPreloadMemory(s.MemoryMaxResults))
	}

	if s.SkillLoadMode != "" && s.SkillLoadMode != "auto" && !biz.IsProgressiveSkillLoad(s.SkillLoadMode) {
		// P2-01: "progressive" is an Aranea-specific composite marker, not a
		// framework SkillLoadMode. The framework's normalizeSkillLoadMode only
		// recognizes "once|turn|session" and silently falls back to "turn" for
		// unknown values. Passing "progressive" here would be misleading because
		// it suggests the framework understands the composite semantic.
		//
		// The progressive semantic is fully expressed by the combination of:
		//   - Default "turn" load mode (the framework default, applied implicitly)
		//   - WithSkillsLoadedContentInToolResults(true) (set above in buildTRPCAgent)
		//   - Directory hints (set above)
		//
		// Skip passing the raw mode to avoid the silent normalization fallback
		// and make the actual load mode explicit. See E2E-P2-01.
		opts = append(opts, trpcllmagent.WithSkillLoadMode(s.SkillLoadMode))
	}

	if s.OutputSchemaJSON != "" {
		var schema map[string]any
		if err := json.Unmarshal([]byte(s.OutputSchemaJSON), &schema); err == nil && len(schema) > 0 {
			opts = append(opts, trpcllmagent.WithOutputSchema(schema))
		}
	}

	if !skipRuntimeModelSelector && s.ModelSelector != "" && s.ModelSelector != "default" {
		selector := buildModelSelector(s.ModelSelector, prov, mod, catalog, rt, lg)
		if selector != nil {
			opts = append(opts, trpcllmagent.WithModelSelector(selector))
		}
	}

	return opts
}

func buildModelSelector(selector string, prov, mod string, catalog biz.TeamModelCatalog, rt *provider.RoundTrip, lg loggateway.Logger) trpcagent.ModelSelector {
	switch selector {
	case "cost-aware":
		return CostAwareModelSelector(prov, mod, catalog, rt, lg)
	case "quality-aware":
		return QualityAwareModelSelector(prov, mod, catalog, rt, lg)
	case "latency-aware":
		return LatencyAwareModelSelector(prov, mod, catalog, rt, lg)
	case "cascade":
		// P2-1：standalone agent 的 cascade 模式 = 自我降为成本档
		// （无 leader 集合——团队场景的分档经 RunOption selector 注入）。
		return CascadeModelSelector(nil, "", "", catalog, rt, lg)
	case "auto", "default", "":
		return nil // defer to framework default
	default:
		// Unknown selector values are treated as "auto" (defer to framework default).
		// This preserves forward-compatibility: if a new selector name is introduced
		// in the settings UI before this switch is updated, the agent still works.
		return nil
	}
}

// shouldInjectCategoryResponsibility returns true when:
//  1. PGO_CATEGORY_RESPONSIBILITY_INJECT env flag is on, AND
//  2. the agent has a PositionID, AND
//  3. the agent has NOT explicitly opted out via metadata_json.
//
// PGO-1-AGENT-02.
func shouldInjectCategoryResponsibility(ag biz.Agent) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("PGO_CATEGORY_RESPONSIBILITY_INJECT")))
	if v != "1" && v != "true" && v != "yes" {
		return false
	}
	if strings.TrimSpace(ag.PositionID) == "" {
		return false
	}
	return !ag.SkipCategoryResponsibility()
}

func ParseVariablesJSON(raw string, lg loggateway.Logger) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		if lg != nil {
			lg.Warn("VariablesJSON 解析失败", loggateway.Err(err), loggateway.Str("raw", strutil.TruncateRunes(raw, 128)))
		}
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// resolveBuildDefaultModel finds a system default provider/model for agents
// that have no model configured. It tries the system RefineLLM setting first,
// then falls back to the first enabled model in the catalog.
func resolveBuildDefaultModel(ctx context.Context, deps TRPCBuilderDeps, lg loggateway.Logger) (string, string) {
	if deps.Sys != nil {
		if refine, err := deps.Sys.GetRefineLLM(ctx); err == nil && refine.Provider != "" && refine.Model != "" {
			return refine.Provider, refine.Model
		}
	}
	if deps.ModelCatalog != nil {
		if models, err := deps.ModelCatalog.List(ctx); err == nil {
			for _, m := range models {
				if m.Enabled && m.Provider != "" && m.Model != "" {
					return m.Provider, m.Model
				}
			}
		}
	}
	lg.Warn("无法解析系统默认模型：RefineLLM 和模型目录均无可用模型",
		loggateway.StepID("agent.build_default_model_fail"))
	return "", ""
}

// plannerKind extracts the PlannerKind from agent settings, defaulting to "".
func plannerKind(ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	return ag.Settings.PlannerKind
}

func plannerConfigJSON(ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	return ag.Settings.PlannerConfigJSON
}

// ResolveModelForRunOption resolves a provider/model pair into a trpcmodel.Model
// suitable for use as an agent.WithModel() RunOption. This enables per-request
// model overrides without baking Provider/Model into the agent build cache key.
func ResolveModelForRunOption(ctx context.Context, deps TRPCBuilderDeps, prov, mod string, lg loggateway.Logger) (trpcmodel.Model, error) {
	return provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, prov, mod, lg)
}

// ResolveAgentBuildModel resolves the same model that the cached agent build
// uses (agent's own provider/model, falling back to the system default when
// unset or unavailable). Business-layer wrappers around the built agent (e.g.
// the graph agent-node summary fallback) use it so auxiliary LLM calls run on
// the same model as the agent itself.
func ResolveAgentBuildModel(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcmodel.Model, error) {
	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	if prov == "" || mod == "" {
		prov, mod = resolveBuildDefaultModel(ctx, deps, lg)
	}
	if prov == "" || mod == "" {
		return nil, apierror.BadRequest(apierror.DomainAgent, "agent provider and model required (no system default available)")
	}
	m, err := provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, prov, mod, lg)
	if err != nil && errors.Is(err, biz.ErrProviderModelNotFound) {
		if fbProv, fbMod := resolveBuildDefaultModel(ctx, deps, lg); fbProv != "" && fbMod != "" {
			m, err = provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, fbProv, fbMod, lg)
		}
	}
	return m, err
}

// isToolPipeEligible determines whether a tool should receive the result_filter
// capability from ToolPipe Extension. Only tools that produce large or structured
// output benefit from filtering; framework-managed tools (todo, memory, skill_*)
// and control-flow tools (transfer_to_agent, await_user_reply) are excluded.
// The framework's own isFrameworkTool check provides additional protection.
func isToolPipeEligible(t trpctool.Tool) bool {
	name := t.Declaration().Name
	// MCP tools are the primary target: they often return large structured data.
	if strings.HasPrefix(name, "mcp_") {
		return true
	}
	// Known long-output tools that benefit from result filtering.
	switch name {
	case "read_file", "execute_command", "web_fetch", "list_directory",
		"search_files", "search_code":
		return true
	}
	return false
}

// hasToolByName reports whether the tool slice contains a tool whose
// Declaration().Name matches the given name. Returns false for empty slices
// or nil declarations.
func hasToolByName(tools []trpctool.Tool, name string) bool {
	for _, t := range tools {
		if decl := t.Declaration(); decl != nil && decl.Name == name {
			return true
		}
	}
	return false
}
