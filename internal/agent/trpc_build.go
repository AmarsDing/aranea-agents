package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/agent/a2ui"
	localexec "aranea-agents/internal/agent/codeexecutor"
	agentplanner "aranea-agents/internal/agent/planner"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/skill/storage"
	skilltrpc "aranea-agents/internal/skill/trpc"
	"aranea-agents/internal/tools/skillruntime"
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

func BuildTRPCLLMAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcagent.Agent, error) {
	if strings.TrimSpace(ag.AgentKey) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgent, "agent_key required")
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
			return nil, apierror.BadRequest(apierror.DomainAgent, "agent provider and model required (no system default available)")
		}
		lg.Info("Agent 无配置模型，使用系统默认模型构建",
			loggateway.StepID("agent.build_default_model"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Str("agent_key", ag.AgentKey),
			loggateway.Str("provider", prov),
			loggateway.Str("model", mod))
	}

	lg.Info("Agent 构建", loggateway.StepID("agent.build"), loggateway.Str("agent_id", ag.ID), loggateway.Str("agent_key", ag.AgentKey), loggateway.Str("provider", prov), loggateway.Str("model", mod))

	m, err := provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, prov, mod, lg)
	if err != nil {
		lg.Error("Agent 构建失败：模型解析", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Str("provider", prov), loggateway.Str("model", mod), loggateway.Err(err))
		return nil, err
	}

	files := ag.Files
	if len(files) == 0 && deps.Agents != nil {
		files, err = deps.Agents.ListAgentPromptFiles(ctx, ag.ID)
		if err != nil {
			lg.Error("Agent 构建失败：提示文件加载", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
			return nil, err
		}
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
		if routerCfg, ok := deps.PluginManager.ModelRouterConfigForAgent(ag.ID); ok {
			hasPluginModelRouter = true
			modelSelectors = append(modelSelectors,
				PluginModelSelector(prov, mod, deps.ModelCatalog, deps.RT, routerCfg, lg),
			)
		}
		if cgCfg, ok := deps.PluginManager.CostGuardConfigForAgent(ag.ID); ok {
			hasPluginCostGuard = true
			modelSelectors = append(modelSelectors,
				PluginCostGuardSelector(prov, mod, deps.ModelCatalog, deps.RT, cgCfg, deps.PluginManager.CostGuardBudgetTrackerForAgent(ag.ID), lg),
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
		trpcllmagent.WithGenerationConfig(trpcmodel.GenerationConfig{Stream: true}),
	)

	var pipeline *a2ui.Pipeline
	if plannerKind(ag) == "a2ui" {
		pipeline = a2ui.NewPipeline(lg)
	}
	if p := agentplanner.Select(deps.DialogMode, plannerKind(ag), plannerConfigJSON(ag), pipeline); p != nil {
		opts = append(opts, trpcllmagent.WithPlanner(p))
	}

	if deps.SkillUC != nil {
		repo, filter, exec, err := buildSkillDeps(ctx, ag, deps)
		if err != nil {
			lg.Error("Agent 构建失败：技能依赖", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
			return nil, err
		}
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
			if skillProfile == "" {
				skillProfile = trpcllmagent.SkillToolProfileKnowledgeOnly
			}
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

	if ts, err := buildToolsetsForAgent(ctx, ag, deps); err != nil {
		lg.Error("Agent 构建失败：工具构建", loggateway.StepID("agent.build_fail"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
		return nil, apierror.Internal(apierror.DomainAgent, "tool build failed").WithCause(err)
	} else if ts != nil {
		if len(ts.ToolSets) > 0 {
			opts = append(opts, trpcllmagent.WithToolSets(ts.ToolSets))
			// WithRefreshToolSetsOnRun is intentionally set to false (disabled).
			// Previously this was true, causing 0.2-5s MCP Initialize+ListTools
			// on every LLM call. Now MCP ToolSets are initialized once during
			// Agent build and cached. When MCP servers change, the agent cache
			// is invalidated (via MCPVersionHash change) and a fresh agent is
			// built with the updated tool list.
		}
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

	if biz.ResolveMemoryRuntimePolicy(ag.Settings).MasterEnabled {
		if !deps.HasMemory {
			lg.Warn("Agent 已启用记忆但未配置 MemoryService，记忆工具已禁用",
				loggateway.StepID("agent.memory_disabled"),
				loggateway.Str("agent_id", ag.ID))
		}
	}

	if chainOpts, cbRegistry := buildCallbackChainOptions(ctx, ag, deps); len(chainOpts) > 0 {
		opts = append(opts, chainOpts...)
		if cbRegistry != nil {
			deps = deps.WithCircuitBreakerRegistry(cbRegistry)
		}
	}

	if ag.Settings != nil {
		opts = append(opts, buildTRPCRuntimeOptions(ag.Settings, hasPluginModelRouter || hasPluginCostGuard, prov, mod, deps.ModelCatalog, deps.RT, lg)...)
		opts = append(opts, SafetyLimitAdapter(ag)...)
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

	return trpcllmagent.New(strings.TrimSpace(ag.AgentKey), opts...), nil
}

// buildSkillDeps resolves the Skill repository, per-invocation visibility filter, and code executor.
// EP-BIZ-01: when SkillDBRepo is injected, the DB repo is the primary backend;
// the local executor falls back to the FS root so skill code files can still be run.
// Layer A + B routing is applied via skillruntime.AgentVisibilityFilter using
// agent_runtime_settings.skill_runtime_json and the turn query in RuntimeState.
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
	filter := skillruntime.NewAgentVisibilityFilter(deps.SkillUC, runtime, deps.Logger(), ag.AgentKey)

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
	}

	if s.MemoryEnabled && s.MemoryMaxResults > 0 {
		opts = append(opts, trpcllmagent.WithPreloadMemory(s.MemoryMaxResults))
	}

	if s.SkillLoadMode != "" && s.SkillLoadMode != "auto" {
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
