package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	mcpconfig "aranea-agents/internal/mcp/config"
	"aranea-agents/internal/skill/storage"
	"aranea-agents/internal/tools"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func buildToolsetsForAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (*tooltrpc.AssembledToolsets, error) {
	var cfg tooltrpc.ToolsetConfig
	var eff map[string]bool

	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		eff = loadEffectiveToolKeys(ctx, deps, ag.ID)
		cfg = tooltrpc.ToolsetConfigFromEffectiveKeys(eff)

		mcpServers, _ := resolveMCPServers(ctx, deps, ag.ID, deps.Logger())
		platformAllowAdHoc := platformMCPAllowAdHocHTTP(ctx, deps)
		if eff[biz.ToolKeyMCPToolSet] && len(mcpServers) > 0 {
			cfg.MCPServers = mcpServers
		}
		if eff[biz.ToolKeyMCPBroker] {
			if len(mcpServers) > 0 {
				if brokerCfg := buildMCPBrokerFromServers(mcpServers, platformAllowAdHoc); brokerCfg != nil {
					cfg.MCPBroker = brokerCfg
				}
			} else {
				mcpBrokerCfg, err := resolveMCPBrokerConfig(ctx, deps, ag.ID)
				if err == nil && mcpBrokerCfg != nil {
					cfg.MCPBroker = mcpBrokerCfg
				}
			}
		}

		cfg.KnowledgeSearch = eff[biz.ToolKeyKnowledgeSearch]
		cfg.KnowledgeReflect = eff[biz.ToolKeyKnowledgeReflect]
		cfg.CallAgent = eff[biz.ToolKeyCallAgent]
		cfg.AwaitHook = deps.AwaitHook

		if ag.Settings.ToolsDeferredJSON != "" {
			var deferred []string
			if err := json.Unmarshal([]byte(ag.Settings.ToolsDeferredJSON), &deferred); err == nil {
				cfg.DeferredTools = deferred
			}
		}
	}

	if len(deps.CustomTools) > 0 {
		cfg.CustomTools = append(cfg.CustomTools, deps.CustomTools...)
	}

	if kanbanpkg.Enabled() {
		if deps.KanbanBridge != nil {
			cfg.Kanban = true
			cfg.KanbanBridge = deps.KanbanBridge
		} else {
			event.CtxFlowLogWarn(ctx, "agent.tool_build", "kanban 已启用但 KanbanBridge 未注入，跳过看板工具",
				event.P("agent_id", ag.ID))
		}
	}

	if deps.HasMemory && biz.ResolveMemoryRuntimePolicy(ag.Settings).MasterEnabled {
		cfg.MemoryEnabled = true
		if deps.MemoryService != nil {
			cfg.MemoryTools = filterMemoryTools(deps.MemoryService.Tools())
		}
	}

	if deps.ToolResultGate != nil {
		cfg.BlobReader = deps.ToolResultGate.BlobReader()
	}

	if !tooltrpc.ToolsetConfigHasAny(cfg) {
		event.CtxFlowLogDone(ctx, "agent.tool_build", "工具构建：未启用任何工具", event.P("agent_id", ag.ID))
		return nil, nil
	}
	event.CtxFlowLogDone(ctx, "agent.tool_build", "工具构建中", event.P("agent_id", ag.ID), event.P("filesystem", cfg.Filesystem), event.P("mcp_servers", len(cfg.MCPServers)))
	applyRuntimeToolConfigs(ctx, ag.ID, eff, deps, &cfg)
	applyWebResearchPlatformDefaults(ctx, deps, &cfg)
	tooltrpc.ResolveGeminiFetchModel(&cfg, ag.Provider, ag.Model)
	if skipped := tooltrpc.PruneUnconfiguredToolFlags(&cfg); len(skipped) > 0 {
		event.CtxFlowLogWarn(ctx, "agent.tool_build", "已跳过未配置凭证的工具，避免构建失败",
			event.P("agent_id", ag.ID), event.P("skipped_tools", skipped))
	}
	if err := applyToolWorkspaceDirs(ctx, ag, deps, &cfg); err != nil {
		event.CtxFlowLogError(ctx, "agent.tool_build", "工具构建失败", event.P("agent_id", ag.ID), event.P("error", err))
		return nil, err
	}
	ts, err := tooltrpc.BuildToolsets(ctx, cfg, deps.LG)
	if err != nil || ts == nil {
		event.CtxFlowLogError(ctx, "agent.tool_build", "工具构建失败", event.P("agent_id", ag.ID), event.P("error", err))
		return ts, err
	}
	toolCount := len(ts.Tools) + len(ts.ToolSets)
	event.CtxFlowLogDone(ctx, "agent.tool_build", "工具构建完成", event.P("agent_id", ag.ID), event.P("tool_count", toolCount))
	if gate := buildToolConfirmGate(ctx, ag, deps); gate != nil {
		tooltrpc.ApplyConfirmationPolicy(ts, gate.confirmationMap())
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

func applyWebResearchPlatformDefaults(ctx context.Context, deps TRPCBuilderDeps, cfg *tooltrpc.ToolsetConfig) {
	if cfg == nil || !cfg.WebResearch {
		return
	}
	if cfg.WebResearchCfg.Ready() {
		return
	}
	if deps.Sys != nil {
		if full, err := deps.Sys.GetWebResearch(ctx); err == nil {
			cfg.WebResearchCfg = tooltrpc.MergeWebResearchConfig(cfg.WebResearchCfg, full)
			if cfg.WebResearchCfg.Ready() {
				return
			}
		}
	}
	patched := tooltrpc.WebResearchConfigFromEnv(cfg.WebResearchCfg)
	if patched.Ready() {
		cfg.WebResearchCfg = patched
	}
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

func applyToolWorkspaceDirs(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, cfg *tooltrpc.ToolsetConfig) error {
	if cfg == nil {
		return nil
	}
	if !cfg.Filesystem && !cfg.ShellExec && !cfg.ClaudeCode {
		return nil
	}
	configured := strings.TrimSpace(cfg.FilesystemDir)
	root, err := resolveToolWorkspaceRoot(ctx, ag, deps, configured)
	if err != nil {
		return err
	}
	if cfg.Filesystem {
		cfg.FilesystemDir = root
	}
	if cfg.ShellExec {
		shellDir := strings.TrimSpace(cfg.ShellExecDir)
		if shellDir == "" {
			cfg.ShellExecDir = root
		} else if err := ensureToolWorkspaceDir(shellDir); err != nil {
			event.CtxFlowLogWarn(ctx, "agent.tool_build", "Shell 工作目录无效，回退到统一工作区",
				event.P("agent_id", ag.ID), event.P("configured_dir", shellDir), event.P("error", err))
			cfg.ShellExecDir = root
		}
	}
	if cfg.ClaudeCode && strings.TrimSpace(cfg.ClaudeCodeDir) == "" {
		cfg.ClaudeCodeDir = root
	}
	return nil
}

func resolveToolWorkspaceRoot(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, configured string) (string, error) {
	return resolveAgentFilesystemDir(ctx, ag, deps, configured)
}

func resolveAgentFilesystemDir(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if err := ensureToolWorkspaceDir(configured); err != nil {
			event.CtxFlowLogWarn(ctx, "agent.tool_build", "工具工作区路径无效，回退到默认目录",
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
	if err := ensureToolWorkspaceDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func ensureToolWorkspaceDir(dir string) error {
	return ensureFilesystemWorkspaceDir(dir)
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

func resolveMCPServers(ctx context.Context, deps TRPCBuilderDeps, agentID string, lg loggateway.Logger) ([]tooltrpc.MCPServerConfig, error) {
	if deps.MCPTooling == nil {
		return nil, nil
	}
	servers, err := deps.MCPTooling.EffectiveServersForAgent(ctx, agentID)
	if err != nil || len(servers) == 0 {
		return nil, err
	}
	out := make([]tooltrpc.MCPServerConfig, 0, len(servers))
	platformAllowAdHoc := platformMCPAllowAdHocHTTP(ctx, deps)
	for _, s := range servers {
		key := strings.TrimSpace(s.ServerKey)
		if key == "" {
			key = strings.TrimSpace(s.ID)
		}
		cfgJSON := strings.TrimSpace(s.ConfigJSON)
		if cfgJSON == "" {
			continue
		}
		sc, err := mcpconfig.ParseServerConfigJSON(cfgJSON)
		if err != nil {
			lg.Warn("MCP server config parse failed", loggateway.StepID("agent.tool_build"), loggateway.Str("server_key", key), loggateway.Err(err))
			continue
		}
		out = append(out, tooltrpc.MCPServerConfig{
			Name:                key,
			Transport:           string(sc.Transport),
			ServerURL:           sc.URL,
			Command:             sc.Command,
			Args:                sc.Args,
			Env:                 sc.Env,
			Headers:             applyMCPAuthHeaders(ctx, key, sc, deps),
			TimeoutSec:          normalizeMCPServerTimeout(sc.TimeoutSec),
			ToolPrefix:          sc.ToolPrefix,
			SessionReconnectMax: sc.SessionReconnectMax,
			AllowAdHocHTTP:      tools.ProductionAllowAdHocHTTP(sc.AllowAdHocHTTP, platformAllowAdHoc),
			AdHocTimeoutSec:     normalizeMCPServerTimeout(sc.AdHocTimeoutSec),
		})
	}
	return out, nil
}

func resolveMCPBrokerConfig(ctx context.Context, deps TRPCBuilderDeps, agentID string) (*tooltrpc.MCPBrokerConfig, error) {
	servers, err := resolveMCPServers(ctx, deps, agentID, deps.Logger())
	if err != nil || len(servers) == 0 {
		return nil, err
	}
	return buildMCPBrokerFromServers(servers, platformMCPAllowAdHocHTTP(ctx, deps)), nil
}

func platformMCPAllowAdHocHTTP(ctx context.Context, deps TRPCBuilderDeps) bool {
	if deps.Sys == nil {
		return false
	}
	s, err := deps.Sys.Get(ctx)
	if err != nil {
		return false
	}
	return s.MCPAllowAdHocHTTP
}

func buildMCPBrokerFromServers(servers []tooltrpc.MCPServerConfig, platformAllowAdHoc bool) *tooltrpc.MCPBrokerConfig {
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
		AllowAdHocHTTP:  tools.ProductionAllowAdHocHTTP(allowAdHoc, platformAllowAdHoc),
		AdHocTimeoutSec: adHocTimeout,
	}
}

func sessionUserID(ctx context.Context) string {
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
		return strings.TrimSpace(inv.Session.UserID)
	}
	return ""
}

func applyMCPAuthHeaders(ctx context.Context, serverKey string, sc mcpconfig.ServerConfig, deps TRPCBuilderDeps) map[string]string {
	headers := make(map[string]string, len(sc.Headers)+1)
	for k, v := range sc.Headers {
		headers[k] = v
	}
	if sc.RequireUserCredentials && deps.MCPTooling != nil && deps.MCPTooling.MCP() != nil {
		bizSC := biz.MCPServerConfig{
			Headers:                sc.Headers,
			Auth:                   biz.MCPAuthConfig{Type: sc.Auth.Type, HeaderName: sc.Auth.HeaderName},
			RequireUserCredentials: sc.RequireUserCredentials,
		}
		if merged, err := deps.MCPTooling.MCP().ResolveUserAuthHeaders(ctx, serverKey, sessionUserID(ctx), bizSC); err == nil {
			for k, v := range merged {
				headers[k] = v
			}
		}
	}
	authType := strings.ToLower(strings.TrimSpace(sc.Auth.Type))
	key := strings.TrimSpace(sc.Auth.APIKey)
	if strings.HasPrefix(authType, "oauth2") {
		if token, err := ResolveMCPAuthToken(ctx, sc.Auth); err == nil && token != "" {
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

func normalizeMCPServerTimeout(sec int) int {
	if sec <= 0 {
		return tools.DefaultMCPServerTimeoutSec
	}
	return sec
}

func filterMemoryTools(tools []trpctool.Tool) []trpctool.Tool {
	filtered := make([]trpctool.Tool, 0, len(tools))
	for _, t := range tools {
		if d := t.Declaration(); d != nil && d.Name == "memory_clear" {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}
