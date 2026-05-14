package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/pkg/skillstorage"
	"aranea-agents/internal/provider"
	skilltrpc "aranea-agents/internal/skill/trpc"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcbuiltin "trpc.group/trpc-go/trpc-agent-go/planner/builtin"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type TRPCBuilderDeps struct {
	Catalog    *biz.LlmProviderModelUsecase
	AgentUC    *biz.AgentUsecase
	Agents     biz.AgentRepository
	RT         *provider.RoundTrip
	SkillUC    *biz.SkillUsecase
	MCPTooling *biz.AgentMCPTooling
	Sys        biz.SystemSettingRepo
	Provider   string
	Model      string
	DialogMode string
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

	opts := []trpcllmagent.Option{
		trpcllmagent.WithModel(m),
		trpcllmagent.WithInstruction(sys),
		trpcllmagent.WithDescription(strings.TrimSpace(ag.DisplayName)),
		trpcllmagent.WithChannelBufferSize(256),
	}

	if strings.EqualFold(deps.DialogMode, "plan") {
		opts = append(opts, trpcllmagent.WithPlanner(trpcbuiltin.New(trpcbuiltin.Options{})))
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

	if ts, err := buildToolsetsForAgent(ctx, ag, deps); err == nil && ts != nil {
		if len(ts.ToolSets) > 0 {
			opts = append(opts, trpcllmagent.WithToolSets(ts.ToolSets))
		}
		if len(ts.Tools) > 0 {
			opts = append(opts, trpcllmagent.WithTools(ts.Tools))
		}
	}

	if ag.Settings != nil {
		opts = append(opts, buildTRPCRuntimeOptions(ag.Settings)...)

		if toolFilter := buildToolFilter(ag.Settings); toolFilter != nil {
			opts = append(opts, trpcllmagent.WithToolFilter(toolFilter))
		}

		if callbacks := buildToolCallbacks(ag.Settings); callbacks != nil {
			opts = append(opts, trpcllmagent.WithToolCallbacks(callbacks))
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

func buildSkillDeps(ctx context.Context, deps TRPCBuilderDeps) (trpcskill.Repository, trpcskill.VisibilityFilter, codeexecutor.CodeExecutor, error) {
	slugs, err := deps.SkillUC.ListEnabledPublishedSkillKeys(ctx)
	if err != nil || len(slugs) == 0 {
		return nil, nil, nil, err
	}

	rootDir := skillstorage.ResolveRoot()
	if deps.Sys != nil {
		if st, e := deps.Sys.Get(ctx); e == nil {
			rootDir = skillstorage.ResolveRootWithPlatform(st.RootDirectory)
		}
	}

	repo, err := skilltrpc.NewFSRepositoryAdapter(rootDir)
	if err != nil {
		return nil, nil, nil, err
	}

	allowSet := strutil.SliceToSet(slugs)
	filter := func(_ context.Context, summary trpcskill.Summary) bool {
		name := strings.TrimSpace(strings.ToLower(summary.Name))
		return allowSet[name]
	}

	exec := skilltrpc.NewLocalExecutor(rootDir)
	return repo, filter, exec, nil
}

func buildToolsetsForAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (*tooltrpc.AssembledToolsets, error) {
	cfg := tooltrpc.ToolsetConfig{}
	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		eff := loadEffectiveToolKeys(deps, ag.ID)
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

		if eff[biz.ToolKeyMCPToolSet] {
			mcpServers, err := resolveMCPServers(ctx, deps, ag.ID)
			if err == nil && len(mcpServers) > 0 {
				cfg.MCPServers = mcpServers
			}
		}

		if eff[biz.ToolKeyMCPBroker] {
			mcpBrokerCfg, err := resolveMCPBrokerConfig(ctx, deps, ag.ID)
			if err == nil && mcpBrokerCfg != nil {
				cfg.MCPBroker = mcpBrokerCfg
			}
		}
	}
	if !cfg.Filesystem && !cfg.ShellExec && !cfg.WebFetch && !cfg.WebSearch &&
		!cfg.GeminiFetch && !cfg.GoogleSearch && !cfg.ArxivSearch && !cfg.Wikipedia &&
		!cfg.Email && !cfg.Todo && !cfg.AwaitReply && !cfg.ClaudeCode && !cfg.WorkspaceExec &&
		len(cfg.MCPServers) == 0 && cfg.MCPBroker == nil {
		return nil, nil
	}
	return tooltrpc.BuildToolsets(ctx, cfg)
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
			Name:       key,
			Transport:  sc.Transport,
			ServerURL:  sc.URL,
			Command:    sc.Command,
			Args:       sc.Args,
			Headers:    sc.Headers,
			TimeoutSec: sc.TimeoutSec,
			ToolPrefix: sc.ToolPrefix,
		})
	}
	return out, nil
}

func resolveMCPBrokerConfig(ctx context.Context, deps TRPCBuilderDeps, agentID string) (*tooltrpc.MCPBrokerConfig, error) {
	if deps.MCPTooling == nil {
		return nil, nil
	}
	servers, err := deps.MCPTooling.EffectiveServersForAgent(ctx, agentID)
	if err != nil || len(servers) == 0 {
		return nil, err
	}
	brokerServers := make([]tooltrpc.MCPServerConfig, 0, len(servers))
	var allowAdHoc bool
	var adHocTimeout int
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
		brokerServers = append(brokerServers, tooltrpc.MCPServerConfig{
			Name:       key,
			Transport:  sc.Transport,
			ServerURL:  sc.URL,
			Command:    sc.Command,
			Args:       sc.Args,
			Headers:    sc.Headers,
			TimeoutSec: sc.TimeoutSec,
		})
		if sc.AllowAdHocHTTP {
			allowAdHoc = true
		}
		if sc.AdHocTimeoutSec > adHocTimeout {
			adHocTimeout = sc.AdHocTimeoutSec
		}
	}
	if len(brokerServers) == 0 && !allowAdHoc {
		return nil, nil
	}
	return &tooltrpc.MCPBrokerConfig{
		Servers:         brokerServers,
		AllowAdHocHTTP:  allowAdHoc,
		AdHocTimeoutSec: adHocTimeout,
	}, nil
}

type mcpServerConfigJSON struct {
	Transport       string            `json:"transport"`
	URL             string            `json:"url"`
	Command         string            `json:"command"`
	Args            []string          `json:"args"`
	Headers         map[string]string `json:"headers"`
	ToolPrefix      string            `json:"tool_prefix"`
	TimeoutSec      int               `json:"timeout_sec"`
	AllowAdHocHTTP  bool              `json:"allow_adhoc_http"`
	AdHocTimeoutSec int               `json:"adhoc_timeout_sec"`
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

func loadEffectiveToolKeys(deps TRPCBuilderDeps, agentID string) map[string]bool {
	m := map[string]bool{}
	if deps.AgentUC == nil || strings.TrimSpace(agentID) == "" {
		return m
	}
	eff, err := deps.AgentUC.GetEffectiveTools(context.Background(), agentID)
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

func buildTRPCRuntimeOptions(s *biz.AgentRuntimeSettings) []trpcllmagent.Option {
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

	if s.ModelSelector != "" && s.ModelSelector != "default" {
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

func buildToolCallbacks(s *biz.AgentRuntimeSettings) *trpctool.Callbacks {
	if !s.ToolsEnabled {
		return nil
	}
	callbacks := trpctool.NewCallbacks()

	callbacks.RegisterAfterTool(func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args.Error != nil {
			return nil, nil
		}
		return &trpctool.AfterToolResult{}, nil
	})

	return callbacks
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
