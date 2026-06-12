package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	agentplanner "aranea-agents/internal/agent/planner"
	"aranea-agents/internal/agent/a2ui"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/skill/storage"
	skilltrpc "aranea-agents/internal/skill/trpc"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func BuildTRPCLLMAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcagent.Agent, error) {
	if strings.TrimSpace(ag.AgentKey) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgent, "agent_key required")
	}
	prov := strutil.FirstNonEmpty(deps.Provider, ag.Provider)
	mod := strutil.FirstNonEmpty(deps.Model, ag.Model)
	if prov == "" || mod == "" {
		return nil, apierror.BadRequest(apierror.DomainAgent, "provider and model required")
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
		}
		if len(ts.Tools) > 0 {
			opts = append(opts, trpcllmagent.WithTools(ts.Tools))
		}
		if ts.DeferredManager != nil {
			deps = deps.WithDeferredManager(ts.DeferredManager)
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
