package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	mcpconfig "aranea-agents/internal/mcp/config"
	"aranea-agents/internal/skill/storage"
	"aranea-agents/internal/tools"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcmcpbroker "trpc.group/trpc-go/trpc-agent-go/tool/mcpbroker"
)

// buildToolsetsForAgent assembles runtime toolsets (P0-2 阶段A 分片形态)。
// The plan (enabled keys, batch-loaded catalog snapshot, confirmation gate) is
// loaded once per build by BuildTRPCLLMAgent and shared with the callback
// chain, eliminating the previous 3×N+1 GetTool loops.
//
// 流程：computeShardPlan（配置计算与单体路径同序等价，按装配组切片）→
// acquireShardPlan（分片缓存命中即复用，未命中仅重建该分片）→
// mergeShardProducts（治理/延迟/去重/消歧/别名横切处理对并集统一重放）→
// 确认门+装饰器（每次构建重放，策略变更不触发分片重建）。
//
// 返回的 retireUnits 是分片引用占位符（shardHoldToolSet）：调用方将其作为
// 缓存 entry 的 toolSets 持有，entry 换代/驱逐时经 graveyard 在在途 run
// 排空后 Close = 释放分片引用。共享分片产物本体由 shardCache 拥有，永不进
// entry/graveyard。第三返回值为分片计划（P0-2B 面元数据来源）。
func buildToolsetsForAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, plan *toolBuildPlan) (*tooltrpc.AssembledToolsets, []trpctool.ToolSet, *shardPlan, error) {
	lg := deps.Logger()
	eff := plan.eff

	// 计划期：完整复刻单体路径的配置计算（runtime config/平台默认/prune/
	// 工作区目录），随后切分为分片计划。
	phaseStart := time.Now()
	sp, err := computeShardPlan(ctx, ag, deps, plan)
	if err != nil {
		lg.Error("工具构建失败", loggateway.StepID("agent.tool_build"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
		return nil, nil, nil, err
	}
	rtCfgMs := time.Since(phaseStart).Milliseconds()
	if sp == nil {
		lg.Info("工具构建：未启用任何工具", loggateway.StepID("agent.tool_build"), loggateway.Str("flow_status", "done"), loggateway.Str("agent_id", ag.ID))
		return nil, nil, nil, nil
	}
	lg.Info("工具构建中", loggateway.StepID("agent.tool_build"), loggateway.Str("flow_status", "done"), loggateway.Str("agent_id", ag.ID), loggateway.Int("shards", len(sp.specs)), loggateway.Int("mcp_servers", len(sp.mcpIdx)))

	// 获取+合并。
	phaseStart = time.Now()
	prods, releases, err := acquireShardPlan(ctx, globalShardCache, sp)
	if err != nil {
		lg.Error("工具构建失败", loggateway.StepID("agent.tool_build"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
		return nil, nil, nil, err
	}
	ts, err := mergeShardProducts(ctx, sp, prods, releases, lg)
	if err != nil {
		lg.Error("工具构建失败", loggateway.StepID("agent.tool_build"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
		return nil, nil, nil, err
	}
	buildMs := time.Since(phaseStart).Milliseconds()

	// retire 单元：每个未释放的分片引用包装为占位符，随 entry 生命周期释放。
	retireUnits := make([]trpctool.ToolSet, 0, len(sp.specs))
	for i, spec := range sp.specs {
		if releases[i] == nil {
			continue // 治理降级已释放（直连 MCP 分片）
		}
		retireUnits = append(retireUnits, newShardHoldToolSet(spec.id, releases[i]))
	}

	toolCount := len(ts.Tools) + len(ts.ToolSets)
	lg.Info("工具构建完成", loggateway.StepID("agent.tool_build"), loggateway.Str("flow_status", "done"), loggateway.Str("agent_id", ag.ID), loggateway.Int("tool_count", toolCount))
	phaseStart = time.Now()
	if plan.gate != nil {
		tooltrpc.ApplyConfirmationPolicy(ts, plan.gate.confirmationMap(ctx))
	}
	gateMs := time.Since(phaseStart).Milliseconds()
	// P0-G3 + P0-D + P2-E + P2-02: 应用工具装饰器（执行超时 + 结果预算 + 确定性缓存 + 流式预算）。
	// 装饰器包装所有 CallableTool，为每次调用提供 60s 默认超时、10KB 结果截断，
	// 并对 ConcurrentSafe 工具（如 file、read_document）启用确定性缓存。
	// 流式工具（StreamableCall）获得 5min 流式超时 + 1MB 流式字节预算（P2-02）。
	phaseStart = time.Now()
	tools.ApplyDecorators(ts, tools.ToolDecoratorConfig{
		Timeout:       tools.DefaultToolTimeout,
		ResultBudget:  tools.DefaultResultBudget,
		EnableCache:   true,
		Logger:        deps.Logger(),
		StreamTimeout: tools.DefaultStreamTimeout,
		StreamBudget:  tools.DefaultStreamBudget,
	})
	decorMs := time.Since(phaseStart).Milliseconds()
	lg.Info("工具构建子步骤耗时",
		loggateway.StepID("agent.tool_phases"),
		loggateway.Str("agent_id", ag.ID),
		loggateway.Int("eff_tools", len(eff)),
		loggateway.Int64("rt_cfg_ms", rtCfgMs), // 分片形态：含计划期全部配置计算（prune 并入）
		loggateway.Int64("prune_ms", 0),
		loggateway.Int64("build_ms", buildMs), // 分片形态：获取（命中复用/未命中重建）+ 合并重放
		loggateway.Int64("gate_ms", gateMs),
		loggateway.Int64("decor_ms", decorMs),
		loggateway.Int("shards", len(sp.specs)))
	return ts, retireUnits, sp, nil
}

// applyRuntimeToolConfigs merges each enabled tool's runtime config (catalog
// base + agent override) into the toolset config, reading from the per-build
// snapshot. Nil catalog (no ToolUC / nothing enabled) skips the merge.
func applyRuntimeToolConfigs(cfg *tooltrpc.ToolsetConfig, eff map[string]bool, catalog *toolBuildCatalog) {
	if cfg == nil || catalog == nil || len(eff) == 0 {
		return
	}
	tooltrpc.ApplyRuntimeConfigMaps(cfg, catalog.mergedConfigMaps(eff))
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

// effKeysList 将有效工具键 map 转换为排序后的字符串列表。
// 用于 SplitCoreResidentTools 的输入。确定性排序保证缓存前缀稳定。
func effKeysList(eff map[string]bool) []string {
	keys := make([]string, 0, len(eff))
	for k := range eff {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func loadEffectiveToolKeys(ctx context.Context, deps TRPCBuilderDeps, agentID string) map[string]bool {
	m := map[string]bool{}
	// Use cached result when available to avoid redundant GetEffectiveTools DB call.
	if deps.CachedEffectiveTools != nil {
		if !deps.CachedEffectiveTools.ToolsEnabled {
			return m
		}
		for _, it := range deps.CachedEffectiveTools.Items {
			if it.Enabled {
				m[it.ToolKey] = true
			}
		}
		return m
	}
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
		} else {
			shellRoot, shellErr := resolveAgentFilesystemDir(ctx, ag, deps, shellDir)
			if shellErr != nil {
				deps.Logger().Warn("Shell 工作目录无效，回退到统一工作区",
					loggateway.StepID("agent.tool_build"), loggateway.Str("agent_id", ag.ID), loggateway.Str("configured_dir", shellDir), loggateway.Err(shellErr))
				cfg.ShellExecDir = root
			} else {
				cfg.ShellExecDir = shellRoot
			}
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

// resolveAgentFilesystemDir returns the agent tool filesystem root.
// Canonical layout: {base}/workspace/{workspaceID}/{agentKey}
// Configured absolute (or any) paths must stay under the tenant root after
// Abs+EvalSymlinks; host paths outside the root are rejected and fall back
// to the safe agent root (C-04).
func resolveAgentFilesystemDir(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, configured string) (string, error) {
	base := toolWorkspaceBase(ctx, deps)
	wsID := workspace.IDFromContext(ctx)
	if strings.TrimSpace(wsID) == "" {
		wsID = workspace.DefaultWorkspaceID
	}
	tenantRoot := filepath.Join(base, "workspace", wsID)
	agentKey := strings.TrimSpace(ag.AgentKey)
	safeRoot := tenantRoot
	if agentKey != "" {
		safeRoot = filepath.Join(tenantRoot, agentKey)
	}

	configured = strings.TrimSpace(configured)
	if configured != "" {
		contained, err := containPathUnderRoot(configured, tenantRoot)
		if err != nil {
			deps.Logger().Warn("工具工作区路径越界或无效，回退到安全根目录",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Str("configured_dir", configured),
				loggateway.Str("tenant_root", tenantRoot),
				loggateway.Err(err))
		} else if err := ensureFilesystemWorkspaceDir(contained); err != nil {
			deps.Logger().Warn("工具工作区路径无效，回退到安全根目录",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Str("configured_dir", configured),
				loggateway.Err(err))
		} else {
			return contained, nil
		}
	}

	if err := ensureFilesystemWorkspaceDir(safeRoot); err != nil {
		return "", err
	}
	return safeRoot, nil
}

func toolWorkspaceBase(ctx context.Context, deps TRPCBuilderDeps) string {
	base := "."
	if deps.Sys != nil {
		if st, err := deps.Sys.Get(ctx); err == nil && strings.TrimSpace(st.RootDirectory) != "" {
			base = storage.Absolute(st.RootDirectory)
		}
	}
	if v := strings.TrimSpace(os.Getenv("ARANEA_WORKSPACE_ROOT")); v != "" {
		return storage.Absolute(v)
	}
	if v := strings.TrimSpace(os.Getenv("WORKSPACE_ROOT")); v != "" {
		return storage.Absolute(v)
	}
	return storage.Absolute(base)
}

// containPathUnderRoot Abs+EvalSymlinks candidate and requires it under root.
// Rejects host absolute paths that escape the tenant/agent sandbox (C-04).
func containPathUnderRoot(candidate, root string) (string, error) {
	candidate = strings.TrimSpace(candidate)
	root = strings.TrimSpace(root)
	if candidate == "" {
		return "", fmt.Errorf("path is empty")
	}
	if root == "" {
		return "", fmt.Errorf("root is empty")
	}
	absCand, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	if eval, err := filepath.EvalSymlinks(absCand); err == nil && eval != "" {
		absCand = eval
	} else if err != nil {
		if _, statErr := os.Lstat(absCand); statErr == nil {
			return "", fmt.Errorf("cannot resolve symlinks for existing path %q: %w", absCand, err)
		}
		// Path does not exist yet; containment still checked on Abs path.
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	if evalRoot, err := filepath.EvalSymlinks(absRoot); err == nil && evalRoot != "" {
		absRoot = evalRoot
	}
	if !pathHasPrefixDir(absCand, absRoot) {
		return "", fmt.Errorf("path %q is outside workspace root %q", absCand, absRoot)
	}
	return absCand, nil
}

func pathHasPrefixDir(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	sep := string(filepath.Separator)
	rootWithSep := root
	if !strings.HasSuffix(rootWithSep, sep) {
		rootWithSep += sep
	}
	pathWithSep := path
	if !strings.HasSuffix(pathWithSep, sep) {
		pathWithSep += sep
	}
	if filepath.Separator == '\\' {
		return strings.HasPrefix(strings.ToLower(pathWithSep), strings.ToLower(rootWithSep)) ||
			strings.EqualFold(path, root)
	}
	return strings.HasPrefix(pathWithSep, rootWithSep) || path == root
}

func ensureToolWorkspaceDir(dir string) error {
	return ensureFilesystemWorkspaceDir(dir)
}

func ensureFilesystemWorkspaceDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("filesystem workspace dir is empty")
	}
	abs, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return fmt.Errorf("resolve %q: %w", dir, err)
	}
	dir = abs
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
		cfg, ok := mcpToolServerConfig(ctx, deps, s, platformAllowAdHoc, lg)
		if !ok {
			continue
		}
		out = append(out, cfg)
	}
	return out, nil
}

// mcpToolServerConfig converts one effective MCP server row into a runtime
// toolset config. It is the single conversion path shared by agent builds
// (resolveMCPServers) and the startup connection pre-warm
// (PrewarmMCPToolSets) so both produce identical pool keys. Returns ok=false
// when the row is unusable (empty config, decrypt failure, parse failure) —
// the caller skips the row and keeps going.
func mcpToolServerConfig(ctx context.Context, deps TRPCBuilderDeps, s biz.EffectiveMCPServer, platformAllowAdHoc bool, lg loggateway.Logger) (tooltrpc.MCPServerConfig, bool) {
	key := strings.TrimSpace(s.ServerKey)
	if key == "" {
		key = strings.TrimSpace(s.ID)
	}
	cfgJSON := strings.TrimSpace(s.ConfigJSON)
	if cfgJSON == "" {
		return tooltrpc.MCPServerConfig{}, false
	}
	if deps.MCPTooling != nil && deps.MCPTooling.MCP() != nil {
		if dec, decErr := deps.MCPTooling.MCP().PrepareConfigJSONForRuntime(ctx, cfgJSON); decErr != nil {
			lg.Warn("MCP server config decrypt failed", loggateway.StepID("agent.tool_build"), loggateway.Str("server_key", key), loggateway.Err(decErr))
			return tooltrpc.MCPServerConfig{}, false
		} else if strings.TrimSpace(dec) != "" {
			cfgJSON = dec
		}
	}
	sc, err := mcpconfig.ParseServerConfigJSON(cfgJSON)
	if err != nil {
		lg.Warn("MCP server config parse failed", loggateway.StepID("agent.tool_build"), loggateway.Str("server_key", key), loggateway.Err(err))
		return tooltrpc.MCPServerConfig{}, false
	}
	cfg := tooltrpc.MCPServerConfig{
		Name:                   key,
		Transport:              string(sc.Transport),
		ServerURL:              sc.URL,
		Command:                sc.Command,
		Args:                   sc.Args,
		Env:                    sc.Env,
		Headers:                applyMCPAuthHeaders(ctx, key, sc, deps),
		TimeoutSec:             normalizeMCPServerTimeout(sc.TimeoutSec),
		ToolPrefix:             sc.ToolPrefix,
		SessionReconnectMax:    sc.SessionReconnectMax,
		AllowAdHocHTTP:         tools.ProductionAllowAdHocHTTP(sc.AllowAdHocHTTP, platformAllowAdHoc),
		AdHocTimeoutSec:        normalizeMCPServerTimeout(sc.AdHocTimeoutSec),
		RequireUserCredentials: sc.RequireUserCredentials,
		AuthHeaderName:         strings.TrimSpace(sc.Auth.HeaderName),
	}
	if sc.RequireUserCredentials && deps.MCPTooling != nil && deps.MCPTooling.MCP() != nil {
		mcpUC := deps.MCPTooling.MCP()
		serverKey := key
		staticHeaders := cfg.Headers
		authHeaderName := cfg.AuthHeaderName
		cfg.HeaderInjector = func(callCtx context.Context) (map[string]string, error) {
			uid := sessionUserID(callCtx)
			if uid == "" {
				return nil, fmt.Errorf("MCP server %q requires user credentials but invocation has no user", serverKey)
			}
			bizSC := biz.MCPServerConfig{
				Headers:                staticHeaders,
				Auth:                   biz.MCPAuthConfig{HeaderName: authHeaderName},
				RequireUserCredentials: true,
			}
			return mcpUC.ResolveUserAuthHeaders(callCtx, serverKey, uid, bizSC)
		}
	}
	return cfg, true
}

func resolveMCPBrokerConfig(ctx context.Context, deps TRPCBuilderDeps, agentID string) (*tooltrpc.MCPBrokerConfig, error) {
	servers, err := resolveMCPServers(ctx, deps, agentID, deps.Logger())
	if err != nil || len(servers) == 0 {
		return nil, err
	}
	cfg := buildMCPBrokerFromServers(servers, platformMCPAllowAdHocHTTP(ctx, deps))
	if cfg != nil {
		cfg.HeaderInjector = mcpUserCredentialInjector(deps, servers)
	}
	return cfg, nil
}

// mcpUserCredentialInjector returns an Invocation-time header injector for
// servers with RequireUserCredentials (E2E-P1-08). Build-time resolution is
// skipped because Invocation.Session is not available yet.
func mcpUserCredentialInjector(deps TRPCBuilderDeps, servers []tooltrpc.MCPServerConfig) func(context.Context, *trpcmcpbroker.HeaderInjectRequest) (map[string]string, error) {
	needUser := false
	byName := make(map[string]tooltrpc.MCPServerConfig, len(servers))
	for _, s := range servers {
		byName[s.Name] = s
		if s.RequireUserCredentials {
			needUser = true
		}
	}
	if !needUser || deps.MCPTooling == nil || deps.MCPTooling.MCP() == nil {
		return nil
	}
	mcpUC := deps.MCPTooling.MCP()
	return func(ctx context.Context, req *trpcmcpbroker.HeaderInjectRequest) (map[string]string, error) {
		if req == nil || req.IsAdHoc {
			// Never inject user secrets onto model-supplied ad-hoc URLs.
			return nil, nil
		}
		s, ok := byName[strings.TrimSpace(req.Selector)]
		if !ok || !s.RequireUserCredentials {
			return nil, nil
		}
		uid := sessionUserID(ctx)
		if uid == "" {
			return nil, fmt.Errorf("MCP server %q requires user credentials but invocation has no user", s.Name)
		}
		bizSC := biz.MCPServerConfig{
			Headers:                s.Headers,
			Auth:                   biz.MCPAuthConfig{HeaderName: s.AuthHeaderName},
			RequireUserCredentials: true,
		}
		return mcpUC.ResolveUserAuthHeaders(ctx, s.Name, uid, bizSC)
	}
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
	// E2E-P1-08: RequireUserCredentials must be resolved at Invocation/tool-call
	// time via MCPBrokerConfig.HeaderInjector. Agent build has no Invocation yet,
	// so only copy static headers here and skip user + static-auth fallback.
	if sc.RequireUserCredentials {
		return headers
	}
	authType := strings.ToLower(strings.TrimSpace(sc.Auth.Type))
	key := strings.TrimSpace(sc.Auth.APIKey)
	if strings.HasPrefix(authType, "oauth2") {
		token, err := ResolveMCPAuthToken(ctx, serverKey, sc.Auth)
		switch {
		case err == nil && token != "":
			key = token
		case err != nil && authType == "oauth2_static":
			// oauth2_static 守卫：令牌缺失/已过期时禁止回退使用同一个（已过期）的
			// access_token——否则过期凭证被静默继续注入，401 被掩盖成工具调用失败。
			// 留空 key → 不注入 auth header，让连接/探活显式失败以提示重新配置。
			deps.Logger().Warn("MCP auth: oauth2_static token unavailable, auth header skipped",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("server_key", serverKey),
				loggateway.Err(err))
		case strings.TrimSpace(sc.Auth.AccessToken) != "":
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
		} else {
			// Unknown auth type with no header_name: log a warning rather than
			// silently discarding the credential, which would cause auth bypass.
			deps.Logger().Warn("MCP auth: unknown auth type with empty header_name, credential not applied",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("server_key", serverKey),
				loggateway.Str("auth_type", authType))
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
