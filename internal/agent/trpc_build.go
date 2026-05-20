package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"aranea-agents/internal/event"
	"strings"
	"time"

	agentplanner "aranea-agents/internal/agent/planner"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	aramemory "aranea-agents/internal/memory"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/skill/storage"
	"aranea-agents/internal/provider"
	skilltrpc "aranea-agents/internal/skill/trpc"
	memorytool "aranea-agents/internal/tools/memory"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type TRPCBuilderDeps struct {
	Catalog     *biz.LlmProviderModelUsecase
	AgentUC     *biz.AgentUsecase
	Agents      biz.AgentRepository
	RT          *provider.RoundTrip
	SkillUC     *biz.SkillUsecase
	MCPTooling  *biz.AgentMCPTooling
	ToolUC      *biz.ToolUsecase
	Sessions    *biz.SessionUsecase
	Sys         biz.SystemSettingRepo
	Provider    string
	Model       string
	DialogMode  string
	SkillDBRepo trpcskill.Repository
	// AwaitHook is an optional service-level callback that blocks the current
	// agent turn mid-flight until the user sends a reply (EP-RT-02).
	// When set, the service-integrated ServiceTool replaces the framework's
	// built-in await_user_reply tool.
	AwaitHook tooltrpc.ReplyFunc
	// HasMemory indicates whether the runner will be created with a MemoryService.
	// EP-RT-05: memory tools are only injected into the agent when the service is
	// actually available to back them; avoids silent no-ops when memory is disabled.
	HasMemory bool
	// Plugins are runner-level trpc plugins (audit, guardrails). Runner still receives
	// them via WithPlugins; LLMAgent uses the product Callback Chain separately.
	Plugins []trpcplugin.Plugin
	// PluginManager merges hook rules into the product Callback Chain (EP-CB-01 Phase 2).
	PluginManager *plugintrpc.Manager
	// MemoryAdmin is the L0–L4 session memory port for prompt injection (EP-BIZ-04).
	MemoryAdmin aramemory.SessionAdminStore
	// KnowledgeRetriever is injected into tool context for knowledge_search (KN-01).
	KnowledgeRetriever *knowledge.Retriever
}

func BuildTRPCLLMAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
	if strings.TrimSpace(ag.AgentKey) == "" {
		return nil, kerrors.BadRequest("AGENT", "agent_key required")
	}
	prov := strutil.FirstNonEmpty(deps.Provider, ag.Provider)
	mod := strutil.FirstNonEmpty(deps.Model, ag.Model)
	if prov == "" || mod == "" {
		return nil, kerrors.BadRequest("AGENT", "provider and model required")
	}

	m, err := provider.TRPCModelForProviderModel(ctx, deps.Catalog, deps.RT, prov, mod)
	if err != nil {
		return nil, err
	}

	files := ag.Files
	if len(files) == 0 && deps.Agents != nil {
		files, err = deps.Agents.ListAgentPromptFiles(ctx, ag.ID)
		if err != nil {
			return nil, err
		}
	}
	sys := BuildSystemPrompt(ag, files, ag.SystemPromptMode)
	promptDeps := Deps{
		Agents:  deps.Agents,
		AgentUC: deps.AgentUC,
	}
	if cue := RuntimeCapabilityCue(ctx, promptDeps, ag); cue != "" {
		sys = sys + "\n\n" + cue
	}
	if l4 := L4MemoryCue(ctx, deps.MemoryAdmin, ag); l4 != "" {
		sys = sys + "\n\n" + l4
	}

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
				PluginModelSelector(prov, mod, deps.Catalog, deps.RT, routerCfg),
			)
		}
		if cgCfg, ok := deps.PluginManager.CostGuardConfigForAgent(ag.ID); ok {
			hasPluginCostGuard = true
			modelSelectors = append(modelSelectors,
				PluginCostGuardSelector(prov, mod, deps.Catalog, deps.RT, cgCfg),
			)
		}
	}
	if len(modelSelectors) > 0 {
		opts = append(opts, trpcllmagent.WithModelSelector(ChainedModelSelector(modelSelectors...)))
	}
	opts = append(opts,
		trpcllmagent.WithInstruction(sys),
		trpcllmagent.WithDescription(strings.TrimSpace(ag.DisplayName)),
		trpcllmagent.WithChannelBufferSize(256),
		trpcllmagent.WithGenerationConfig(trpcmodel.GenerationConfig{Stream: true}),
	)

	if p := agentplanner.Select(deps.DialogMode, plannerKind(ag)); p != nil {
		opts = append(opts, trpcllmagent.WithPlanner(p))
	}

	if deps.SkillUC != nil {
		repo, filter, exec, err := buildSkillDeps(ctx, deps)
		if err != nil {
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
		opts = append(opts,
			trpcllmagent.WithSkillToolProfile(trpcllmagent.SkillToolProfileFull),
			trpcllmagent.WithSkillsDirectoryHints(true),
		)
	}

	if ts, err := buildToolsetsForAgent(ctx, ag, deps); err != nil {
		return nil, fmt.Errorf("tool build failed: %w", err)
	} else if ts != nil {
		if len(ts.ToolSets) > 0 {
			opts = append(opts, trpcllmagent.WithToolSets(ts.ToolSets))
		}
		if len(ts.Tools) > 0 {
			opts = append(opts, trpcllmagent.WithTools(ts.Tools))
		}
	}

	// EP-RT-05: Memory tools are only injected when both the agent setting is on
	// AND the runner will have a MemoryService to back them.  When HasMemory is
	// false but MemoryEnabled is true, we log a warning so operators can act.
	if ag.Settings != nil && ag.Settings.MemoryEnabled {
		if deps.HasMemory {
			if memTools := memorytool.DefaultTools(); len(memTools) > 0 {
				opts = append(opts, trpcllmagent.WithTools(memTools))
			}
		} else {
			event.CtxFlowLogWarn(ctx, "system.agent.memory_disabled", "Agent 已启用记忆但未配置 MemoryService，记忆工具已禁用",
				event.P("agent_id", ag.ID))
		}
	}

	if chainOpts := buildCallbackChainOptions(ctx, ag, deps); len(chainOpts) > 0 {
		opts = append(opts, chainOpts...)
	}

	if ag.Settings != nil {
		opts = append(opts, buildTRPCRuntimeOptions(ag.Settings, hasPluginModelRouter || hasPluginCostGuard)...)

		if toolFilter := buildToolFilter(ag.Settings); toolFilter != nil {
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

// buildSkillDeps resolves the Skill repository, visibility filter, and code executor.
// EP-BIZ-01: when SkillDBRepo is injected, the DB repo is the primary backend;
// the local executor falls back to the FS root so skill code files can still be run.
func buildSkillDeps(ctx context.Context, deps TRPCBuilderDeps) (trpcskill.Repository, trpcskill.VisibilityFilter, codeexecutor.CodeExecutor, error) {
	slugs, err := deps.SkillUC.ListEnabledPublishedSkillKeys(ctx)
	if err != nil || len(slugs) == 0 {
		event.CtxFlowLogWarn(ctx, "system.agent.skill_build", "技能构建：无可用技能", event.P("error", err), event.P("slug_count", len(slugs)))
		return nil, nil, nil, err
	}
	event.CtxFlowLogDone(ctx, "system.agent.skill_build", "技能构建：解析中", event.P("slug_count", len(slugs)))

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

	allowSet := strutil.SliceToSet(slugs)
	filter := func(_ context.Context, summary trpcskill.Summary) bool {
		name := strings.TrimSpace(strings.ToLower(summary.Name))
		return allowSet[name]
	}

	exec := skilltrpc.NewExecutor(os.Getenv("CODE_EXECUTOR_BACKEND"), rootDir)
	event.CtxFlowLogDone(ctx, "system.agent.skill_build", "技能构建完成", event.P("slug_count", len(slugs)), event.P("repo_type", fmt.Sprintf("%T", repo)))
	return repo, filter, exec, nil
}

func buildToolsetsForAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (*tooltrpc.AssembledToolsets, error) {
	cfg := tooltrpc.ToolsetConfig{}
	var eff map[string]bool
	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		eff = loadEffectiveToolKeys(ctx, deps, ag.ID)
		cfg.Filesystem = eff["read_file"] || eff["read_multiple_files"] || eff["save_file"] || eff["list_file"] || eff["search_file"] || eff["search_content"] || eff["replace_content"]
		cfg.ShellExec = eff["shell_exec"]
		cfg.WebFetch = eff["web_fetch"]
		cfg.WebSearch = eff["duckduckgo_search"]
		cfg.GeminiFetch = eff["gemini_web_fetch"]
		cfg.GoogleSearch = eff["google_search"]
		cfg.ArxivSearch = eff["arxiv_search"]
		cfg.Wikipedia = eff["wikipedia_search"]
		cfg.Email = eff["send_email"]
		cfg.Todo = eff["todo_write"]
		cfg.AwaitReply = eff["await_user_reply"]
		cfg.ClaudeCode = eff["claude_code"]
		cfg.WorkspaceExec = eff["workspace_exec"]

		mcpServers, _ := resolveMCPServers(ctx, deps, ag.ID)
		if eff[biz.ToolKeyMCPToolSet] && len(mcpServers) > 0 {
			cfg.MCPServers = mcpServers
		}
		// Auto-mount MCPBroker when the agent has effective MCP servers (runtime discovery).
		if len(mcpServers) > 0 {
			if brokerCfg := buildMCPBrokerFromServers(mcpServers); brokerCfg != nil {
				cfg.MCPBroker = brokerCfg
			}
		} else if eff[biz.ToolKeyMCPBroker] {
			mcpBrokerCfg, err := resolveMCPBrokerConfig(ctx, deps, ag.ID)
			if err == nil && mcpBrokerCfg != nil {
				cfg.MCPBroker = mcpBrokerCfg
			}
		}

		cfg.KnowledgeSearch = eff[biz.ToolKeyKnowledgeSearch]
		cfg.CallAgent = eff[biz.ToolKeyCallAgent]
		// EP-RT-02: inject the service hook so await_user_reply blocks mid-turn.
		cfg.AwaitHook = deps.AwaitHook
	}
	if !cfg.Filesystem && !cfg.ShellExec && !cfg.WebFetch && !cfg.WebSearch &&
		!cfg.GeminiFetch && !cfg.GoogleSearch && !cfg.ArxivSearch && !cfg.Wikipedia &&
		!cfg.Email && !cfg.Todo && !cfg.AwaitReply && !cfg.ClaudeCode && !cfg.WorkspaceExec &&
		!cfg.KnowledgeSearch && !cfg.CallAgent &&
		len(cfg.MCPServers) == 0 && cfg.MCPBroker == nil {
		event.CtxFlowLogDone(ctx, "system.agent.tool_build", "工具构建：未启用任何工具", event.P("agent_id", ag.ID))
		return nil, nil
	}
	event.CtxFlowLogDone(ctx, "system.agent.tool_build", "工具构建中", event.P("agent_id", ag.ID), event.P("filesystem", cfg.Filesystem), event.P("mcp_servers", len(cfg.MCPServers)))
	applyRuntimeToolConfigs(ctx, ag.ID, eff, deps, &cfg)
	tooltrpc.ResolveGeminiFetchModel(&cfg, ag.Provider, ag.Model)
	if skipped := tooltrpc.PruneUnconfiguredToolFlags(&cfg); len(skipped) > 0 {
		event.CtxFlowLogWarn(ctx, "system.agent.tool_build", "已跳过未配置凭证的工具，避免构建失败",
			event.P("agent_id", ag.ID), event.P("skipped_tools", skipped))
	}
	if cfg.Filesystem {
		dir, err := resolveAgentFilesystemDir(ctx, ag, deps, cfg.FilesystemDir)
		if err != nil {
			event.CtxFlowLogError(ctx, "system.agent.tool_build", "工具构建失败", event.P("agent_id", ag.ID), event.P("error", err))
			return nil, err
		}
		cfg.FilesystemDir = dir
	}
	ts, err := tooltrpc.BuildToolsets(ctx, cfg)
	if err != nil || ts == nil {
		event.CtxFlowLogError(ctx, "system.agent.tool_build", "工具构建失败", event.P("agent_id", ag.ID), event.P("error", err))
		return ts, err
	}
	toolCount := len(ts.Tools) + len(ts.ToolSets)
	event.CtxFlowLogDone(ctx, "system.agent.tool_build", "工具构建完成", event.P("agent_id", ag.ID), event.P("tool_count", toolCount))
	if confirm := buildToolConfirmationPolicy(ctx, ag, deps); len(confirm) > 0 {
		tooltrpc.ApplyConfirmationPolicy(ts, confirm)
	}
	return ts, nil
}

func applyRuntimeToolConfigs(ctx context.Context, agentID string, eff map[string]bool, deps TRPCBuilderDeps, cfg *tooltrpc.ToolsetConfig) {
	if cfg == nil || deps.ToolUC == nil || len(eff) == 0 {
		return
	}
	overrides, err := deps.ToolUC.ListToolAgentOverridesByAgent(ctx, agentID)
	if err != nil {
		overrides = nil
	}
	overrideByKey := make(map[string]biz.ToolAgentOverride, len(overrides))
	for _, o := range overrides {
		overrideByKey[strings.TrimSpace(o.ToolKey)] = o
	}
	merged := make(map[string]map[string]any)
	for key := range eff {
		if !eff[key] {
			continue
		}
		tool, err := deps.ToolUC.GetTool(ctx, key)
		if err != nil {
			continue
		}
		base := strings.TrimSpace(tool.ConfigJSON)
		if base == "" {
			base = strings.TrimSpace(tool.DefaultConfigJSON)
		}
		ov := overrideByKey[key]
		merged[key] = biz.MergeToolConfigJSON(base, ov.ConfigOverrideJSON)
	}
	tooltrpc.ApplyRuntimeConfigMaps(cfg, merged)
}

func resolveAgentFilesystemDir(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if err := ensureFilesystemWorkspaceDir(configured); err != nil {
			event.CtxFlowLogWarn(ctx, "system.agent.tool_build", "工具工作区路径无效，回退到默认目录",
				event.P("agent_id", ag.ID), event.P("configured_dir", configured), event.P("error", err))
		} else {
			return configured, nil
		}
	}
	base := "."
	if deps.Sys != nil {
		if st, err := deps.Sys.Get(ctx); err == nil && strings.TrimSpace(st.RootDirectory) != "" {
			base = storage.Absolute(st.RootDirectory)
		}
	}
	if v := strings.TrimSpace(os.Getenv("ARANEA_WORKSPACE_ROOT")); v != "" {
		base = storage.Absolute(v)
	} else if v := strings.TrimSpace(os.Getenv("WORKSPACE_ROOT")); v != "" {
		base = storage.Absolute(v)
	}
	agentKey := strings.TrimSpace(ag.AgentKey)
	dir := filepath.Join(base, "workspace")
	if agentKey != "" {
		dir = filepath.Join(dir, agentKey)
	}
	if err := ensureFilesystemWorkspaceDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func ensureFilesystemWorkspaceDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("filesystem workspace dir is empty")
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %q: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", dir, err)
	}
	return nil
}

func resolveMCPServers(ctx context.Context, deps TRPCBuilderDeps, agentID string) ([]tooltrpc.MCPServerConfig, error) {
	if deps.MCPTooling == nil {
		return nil, nil
	}
	servers, err := deps.MCPTooling.EffectiveServersForAgent(ctx, agentID)
	if err != nil || len(servers) == 0 {
		return nil, err
	}
	out := make([]tooltrpc.MCPServerConfig, 0, len(servers))
	for _, s := range servers {
		key := strings.TrimSpace(s.ServerKey)
		if key == "" {
			key = strings.TrimSpace(s.ID)
		}
		cfgJSON := strings.TrimSpace(s.ConfigJSON)
		if cfgJSON == "" {
			continue
		}
		sc, err := parseMCPServerConfigJSON(cfgJSON)
		if err != nil {
			continue
		}
		out = append(out, tooltrpc.MCPServerConfig{
			Name:                  key,
			Transport:             sc.Transport,
			ServerURL:             sc.URL,
			Command:               sc.Command,
			Args:                  sc.Args,
			Env:                   sc.Env,
			Headers:               applyMCPAuthHeaders(ctx, sc),
			TimeoutSec:            sc.TimeoutSec,
			ToolPrefix:            sc.ToolPrefix,
			SessionReconnectMax:   sc.SessionReconnectMax,
			AllowAdHocHTTP:        sc.AllowAdHocHTTP,
			AdHocTimeoutSec:       sc.AdHocTimeoutSec,
		})
	}
	return out, nil
}

func resolveMCPBrokerConfig(ctx context.Context, deps TRPCBuilderDeps, agentID string) (*tooltrpc.MCPBrokerConfig, error) {
	servers, err := resolveMCPServers(ctx, deps, agentID)
	if err != nil || len(servers) == 0 {
		return nil, err
	}
	return buildMCPBrokerFromServers(servers), nil
}

func buildMCPBrokerFromServers(servers []tooltrpc.MCPServerConfig) *tooltrpc.MCPBrokerConfig {
	if len(servers) == 0 {
		return nil
	}
	var allowAdHoc bool
	var adHocTimeout int
	for _, s := range servers {
		if s.AllowAdHocHTTP {
			allowAdHoc = true
		}
		if s.AdHocTimeoutSec > adHocTimeout {
			adHocTimeout = s.AdHocTimeoutSec
		}
	}
	return &tooltrpc.MCPBrokerConfig{
		Servers:         servers,
		AllowAdHocHTTP:  allowAdHoc,
		AdHocTimeoutSec: adHocTimeout,
	}
}

type mcpAuthConfigJSON struct {
	Type         string `json:"type"`
	APIKey       string `json:"api_key"`
	HeaderName   string `json:"header_name"`
	TokenURL     string `json:"token_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Scope        string `json:"scope"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type mcpServerConfigJSON struct {
	Transport           string            `json:"transport"`
	URL                 string            `json:"url"`
	Command             string            `json:"command"`
	Args                []string          `json:"args"`
	Env                 map[string]string `json:"env"`
	Headers             map[string]string `json:"headers"`
	Auth                mcpAuthConfigJSON `json:"auth"`
	ToolPrefix          string            `json:"tool_prefix"`
	TimeoutSec          int               `json:"timeout_sec"`
	SessionReconnectMax int               `json:"session_reconnect_max"`
	AllowAdHocHTTP      bool              `json:"allow_adhoc_http"`
	AdHocTimeoutSec     int               `json:"adhoc_timeout_sec"`
}

func applyMCPAuthHeaders(ctx context.Context, sc mcpServerConfigJSON) map[string]string {
	headers := make(map[string]string, len(sc.Headers)+1)
	for k, v := range sc.Headers {
		headers[k] = v
	}
	authType := strings.ToLower(strings.TrimSpace(sc.Auth.Type))
	key := strings.TrimSpace(sc.Auth.APIKey)
	if strings.HasPrefix(authType, "oauth2") {
		if token, err := resolveMCPAuthToken(ctx, sc.Auth); err == nil && token != "" {
			key = token
		} else if strings.TrimSpace(sc.Auth.AccessToken) != "" {
			key = strings.TrimSpace(sc.Auth.AccessToken)
		}
	}
	if key == "" {
		return headers
	}
	headerName := strings.TrimSpace(sc.Auth.HeaderName)
	switch authType {
	case "api_key", "bearer", "", "oauth2", "oauth2_static", "oauth2_client_credentials", "oauth2_refresh":
		if headerName == "" {
			headerName = "Authorization"
		}
		if strings.EqualFold(headerName, "Authorization") && !strings.HasPrefix(strings.ToLower(key), "bearer ") {
			headers[headerName] = "Bearer " + key
		} else {
			headers[headerName] = key
		}
	default:
		if headerName != "" {
			headers[headerName] = key
		}
	}
	return headers
}

func parseMCPServerConfigJSON(raw string) (mcpServerConfigJSON, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var c mcpServerConfigJSON
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return mcpServerConfigJSON{}, err
	}
	return c, nil
}

func loadEffectiveToolKeys(ctx context.Context, deps TRPCBuilderDeps, agentID string) map[string]bool {
	m := map[string]bool{}
	if deps.AgentUC == nil || strings.TrimSpace(agentID) == "" {
		return m
	}
	eff, err := deps.AgentUC.GetEffectiveTools(ctx, agentID)
	if err != nil || !eff.ToolsEnabled {
		return m
	}
	for _, it := range eff.Items {
		if it.Enabled {
			m[it.ToolKey] = true
		}
	}
	return m
}

func buildTRPCRuntimeOptions(s *biz.AgentRuntimeSettings, skipRuntimeModelSelector bool) []trpcllmagent.Option {
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
		if s.L0SummaryKeepTurns > 0 {
			opts = append(opts, trpcllmagent.WithContextCompactionKeepRecentRequests(s.L0SummaryKeepTurns))
		}
	}

	if s.SessionSummaryEnabled {
		opts = append(opts, trpcllmagent.WithAddSessionSummary(true))
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
		selector := buildModelSelector(s.ModelSelector)
		if selector != nil {
			opts = append(opts, trpcllmagent.WithModelSelector(selector))
		}
	}

	return opts
}

func buildModelSelector(selector string) trpcagent.ModelSelector {
	switch selector {
	case "auto":
		return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
			return nil, nil
		}
	default:
		return nil
	}
}

func ParseVariablesJSON(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
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

func buildToolFilter(s *biz.AgentRuntimeSettings) trpctool.FilterFunc {
	denyList := jsonStringList(s.ToolsDenyJSON)
	if len(denyList) == 0 {
		return nil
	}
	return trpctool.NewExcludeToolNamesFilter(denyList...)
}

func recordToolInvocationAfter(ctx context.Context, args *trpctool.AfterToolArgs, ag biz.Agent, deps TRPCBuilderDeps) {
	toolKey := strings.TrimSpace(args.ToolName)
	if toolKey == "" {
		return
	}
	ended := time.Now().UTC()
	started := ended
	var durationMS int
	if t, ok := ctx.Value(toolCallStartKey{}).(time.Time); ok {
		started = t
		durationMS = int(ended.Sub(t).Milliseconds())
	}
	status, errCode, errMsg := invocationStatusFromAfter(args)
	if status == "blocked" && errCode == "confirmation_required" {
		return
	}
	write := biz.ToolInvocationWrite{
		ToolKey:       toolKey,
		Status:        status,
		DurationMS:    durationMS,
		StartedAt:     started.Format(time.RFC3339),
		EndedAt:       ended.Format(time.RFC3339),
		InputPreview:  previewFromArgs(args.Arguments),
		OutputPreview: previewFromResult(args.Result),
		ErrorCode:     errCode,
		ErrorMessage:  errMsg,
		Source:        "adk",
		ToolCallID:    args.ToolCallID,
	}
	recordToolInvocationWrite(ctx, write, ag, deps)
}

func previewFromArgs(args []byte) string {
	if len(args) == 0 {
		return ""
	}
	s := string(args)
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}

func previewFromResult(result any) string {
	if result == nil {
		return ""
	}
	b, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	s := string(b)
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}

func buildToolRetryPolicy(s *biz.AgentRuntimeSettings) *trpctool.RetryPolicy {
	if !s.ToolsEnabled || !s.ToolsRetryEnabled {
		return nil
	}
	maxAttempts := s.ToolsRetryMaxAttempts
	if maxAttempts < 2 {
		maxAttempts = 2
	}
	initialMs := s.ToolsRetryInitialIntervalMs
	if initialMs <= 0 {
		initialMs = 500
	}
	backoff := s.ToolsRetryBackoffFactor
	if backoff <= 0 {
		backoff = 2.0
	}
	maxMs := s.ToolsRetryMaxIntervalMs
	if maxMs <= 0 {
		maxMs = 5000
	}
	return &trpctool.RetryPolicy{
		MaxAttempts:     maxAttempts,
		InitialInterval: time.Duration(initialMs) * time.Millisecond,
		BackoffFactor:   backoff,
		MaxInterval:     time.Duration(maxMs) * time.Millisecond,
		Jitter:          s.ToolsRetryJitter,
		RetryOn:         trpctool.DefaultRetryOn,
	}
}

func jsonStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "{}" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil
	}
	return list
}

// plannerKind extracts the PlannerKind from agent settings, defaulting to "".
func plannerKind(ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	return ag.Settings.PlannerKind
}
