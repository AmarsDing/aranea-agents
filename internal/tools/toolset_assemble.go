package tools

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/outbound"
	"aranea-agents/internal/tools/browser"
	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/internal/tools/codingbridge"
	"aranea-agents/internal/tools/custom"
	"aranea-agents/internal/tools/deferred"
	documentpkg "aranea-agents/internal/tools/document"
	"aranea-agents/internal/tools/filenorm"
	hostexecpkg "aranea-agents/internal/tools/hostexec"
	memorytool "aranea-agents/internal/tools/memory"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcagenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
	trpcclaudecode "trpc.group/trpc-go/trpc-agent-go/tool/claudecode"
	trpcfile "trpc.group/trpc-go/trpc-agent-go/tool/file"
	trpcgooglesearch "trpc.group/trpc-go/trpc-agent-go/tool/google/search"
	trpcopenapi "trpc.group/trpc-go/trpc-agent-go/tool/openapi"
	trpcgeminifetch "trpc.group/trpc-go/trpc-agent-go/tool/webfetch/geminifetch"
)

// assembleContext holds shared state for the Assemble function.
type assembleContext struct {
	ctx         context.Context
	cfg         AssemblyConfig
	out         *AssembledToolsets
	enabled     map[string]bool
	deferredSet map[string]bool
	lg          loggateway.Logger
}

// closeAll releases every ToolSet assembled so far. Called on Assemble error
// paths so pooled MCP connections release their pool references (and
// non-pooled ones close for real) instead of leaking.
func (ac *assembleContext) closeAll() {
	for _, ts := range ac.out.ToolSets {
		if ts == nil {
			continue
		}
		if err := ts.Close(); err != nil {
			ac.lg.Warn("tools.Assemble 错误路径清理 ToolSet 失败",
				loggateway.StepID("tool.assemble.cleanup"),
				loggateway.Str("toolset", ts.Name()),
				loggateway.Err(err))
		}
	}
	ac.out.ToolSets = nil
}

// assembleFromRegistry instantiates tools from the global registry entries.
func (ac *assembleContext) assembleFromRegistry() error {
	for _, reg := range Registry() {
		if !ac.enabled[reg.Name] {
			continue
		}
		// WP-4 修复：不再跳过 deferred 工具，全部正常装配。
		if reg.ToolSetFactory != nil {
			ts, err := reg.ToolSetFactory(ac.ctx)
			if err != nil {
				return apierror.Internal(apierror.DomainTool, fmt.Sprintf("tool %s: %s", reg.Name, err.Error()))
			}
			if ts != nil {
				ac.out.ToolSets = append(ac.out.ToolSets, ts)
			} else if !reg.AssembledElsewhere {
				// 未标记 AssembledElsewhere 却返回 nil 才是真异常；占位条目
				// 的实际装配在后续 phase，静默跳过（见 ToolRegistration 注释）。
				ac.lg.Warn("tools.assemble.factory_nil",
					loggateway.StepID("tool.assemble.factory_nil"),
					loggateway.Str("tool", reg.Name),
					loggateway.Str("reason", "factory returned nil without error"))
			}
		} else if reg.Factory != nil {
			t, err := reg.Factory(ac.ctx)
			if err != nil {
				return apierror.Internal(apierror.DomainTool, fmt.Sprintf("tool %s: %s", reg.Name, err.Error()))
			}
			if t != nil {
				ac.out.Tools = append(ac.out.Tools, t)
			} else if !reg.AssembledElsewhere {
				ac.lg.Warn("tools.assemble.factory_nil",
					loggateway.StepID("tool.assemble.factory_nil"),
					loggateway.Str("tool", reg.Name),
					loggateway.Str("reason", "factory returned nil without error"))
			}
		}
	}
	return nil
}

// assembleBuiltinToolsets creates file, hostexec, read_document, read_spreadsheet tools.
func (ac *assembleContext) assembleBuiltinToolsets() error {
	if ac.enabled["file"] {
		var opts []trpcfile.Option
		if ac.cfg.FilesystemDir != "" {
			opts = append(opts, trpcfile.WithBaseDir(ac.cfg.FilesystemDir))
		}
		ts, err := trpcfile.NewToolSet(opts...)
		if err != nil {
			return apierror.Internal(apierror.DomainTool, "file toolset: "+err.Error())
		}
		ac.out.ToolSets = append(ac.out.ToolSets, wrapFileToolSetWithWorktree(filenorm.WrapToolSet(ts), ac.cfg.FilesystemDir, ac.lg))
	}

	if ac.enabled["hostexec"] {
		ts, err := hostexecpkg.BuildHostexecToolSet(ac.cfg.ShellExec.Dir, ac.cfg.ShellExec.Env)
		if err != nil {
			return apierror.Internal(apierror.DomainTool, "hostexec toolset: "+err.Error())
		}
		ac.out.ToolSets = append(ac.out.ToolSets, ts)
	}

	if ac.enabled["read_document"] {
		ac.out.Tools = append(ac.out.Tools, documentpkg.NewReadDocumentTool(ac.cfg.FilesystemDir))
	}

	if ac.enabled["read_spreadsheet"] {
		ac.out.Tools = append(ac.out.Tools, documentpkg.NewReadSpreadsheetTool(ac.cfg.FilesystemDir))
	}

	return nil
}

// assembleSearchTools creates geminifetch and google_search tools.
func (ac *assembleContext) assembleSearchTools() error {
	if ac.enabled["geminifetch"] {
		if model := strings.TrimSpace(ac.cfg.Search.GeminiModel); model != "" {
			t, err := trpcgeminifetch.NewTool(model)
			if err != nil {
				return apierror.Internal(apierror.DomainTool, "geminifetch: "+err.Error())
			}
			ac.out.Tools = append(ac.out.Tools, t)
		} else {
			ac.lg.Warn("tools.assemble.geminifetch_no_model",
				loggateway.StepID("tool.assemble.geminifetch_no_model"),
				loggateway.Str("reason", "gemini_model config is empty"))
		}
	}

	if ac.enabled["google_search"] {
		apiKey := strings.TrimSpace(ac.cfg.Search.GoogleAPIKey)
		cx := strings.TrimSpace(ac.cfg.Search.GoogleCX)
		if apiKey != "" && cx != "" {
			ts, err := trpcgooglesearch.NewToolSet(ac.ctx,
				trpcgooglesearch.WithAPIKey(apiKey),
				trpcgooglesearch.WithEngineID(cx),
			)
			if err != nil {
				return apierror.Internal(apierror.DomainTool, "google search: "+err.Error())
			}
			ac.out.ToolSets = append(ac.out.ToolSets, ts)
		} else {
			ac.lg.Warn("tools.assemble.google_search_no_config",
				loggateway.StepID("tool.assemble.google_search_no_config"),
				loggateway.Str("reason", "api_key or cx is empty"))
		}
	}

	return nil
}

// assembleClaudeCodeToolset creates the claudecode toolset with sandbox.
func (ac *assembleContext) assembleClaudeCodeToolset() error {
	if !ac.enabled["claudecode"] {
		return nil
	}

	var opts []trpcclaudecode.Option
	if ac.cfg.ClaudeCode.Dir != "" {
		opts = append(opts, trpcclaudecode.WithBaseDir(ac.cfg.ClaudeCode.Dir))
	}
	if ac.cfg.ClaudeCode.ReadOnly {
		opts = append(opts, trpcclaudecode.WithReadOnly(true))
	}
	if ac.cfg.ClaudeCode.MaxFileSize > 0 {
		opts = append(opts, trpcclaudecode.WithMaxFileSize(ac.cfg.ClaudeCode.MaxFileSize))
	}
	if ac.cfg.ClaudeCode.WebFetch != nil {
		opts = append(opts, trpcclaudecode.WithWebFetchOptions(trpcclaudecode.WebFetchOptions{
			AllowAll:         ac.cfg.ClaudeCode.WebFetch.AllowAll,
			AllowedDomains:   ac.cfg.ClaudeCode.WebFetch.AllowedDomains,
			BlockedDomains:   ac.cfg.ClaudeCode.WebFetch.BlockedDomains,
			Timeout:          ac.cfg.ClaudeCode.WebFetch.Timeout,
			MaxContentLength: ac.cfg.ClaudeCode.WebFetch.MaxContentLength,
		}))
	}
	if ac.cfg.ClaudeCode.WebSearch != nil {
		opts = append(opts, trpcclaudecode.WithWebSearchOptions(trpcclaudecode.WebSearchOptions{
			Provider: ac.cfg.ClaudeCode.WebSearch.Provider,
			BaseURL:  ac.cfg.ClaudeCode.WebSearch.BaseURL,
			APIKey:   ac.cfg.ClaudeCode.WebSearch.APIKey,
			EngineID: ac.cfg.ClaudeCode.WebSearch.EngineID,
		}))
	}

	ts, err := trpcclaudecode.NewToolSet(opts...)
	if err != nil {
		return apierror.Internal(apierror.DomainTool, "claudecode: "+err.Error())
	}
	if len(ac.cfg.ClaudeCode.CommandAllowList) > 0 {
		ts = SandboxedToolSet(ts, ClaudeCodeSandboxConfig{
			CommandAllowList: ac.cfg.ClaudeCode.CommandAllowList,
		})
	}
	ac.out.ToolSets = append(ac.out.ToolSets, ts)
	return nil
}

// assembleOpenAPIToolsets creates OpenAPI toolsets from spec configs.
func (ac *assembleContext) assembleOpenAPIToolsets() error {
	for _, spec := range ac.cfg.OpenAPISpecs {
		var specLoader trpcopenapi.Loader
		var err error
		if len(spec.SpecData) > 0 {
			specLoader, err = trpcopenapi.NewDataLoader(spec.SpecData)
		} else if spec.SpecURL != "" {
			specLoader, err = trpcopenapi.NewURILoader(spec.SpecURL)
		}
		// TPM-P2-02: log loader failures so misconfigurations are visible instead
		// of silently disappearing. We continue rather than aborting all assembly
		// so a bad spec doesn't block unrelated toolsets.
		if err != nil {
			ac.lg.Warn("tools.assemble.openapi_loader_failed",
				loggateway.StepID("tool.assemble.openapi_loader_fail"),
				loggateway.Str("spec_name", spec.Name),
				loggateway.Str("spec_url", spec.SpecURL),
				loggateway.Err(err))
			continue
		}
		if specLoader == nil {
			continue
		}
		ts, err := trpcopenapi.NewToolSet(ac.ctx,
			trpcopenapi.WithSpecLoader(specLoader),
			trpcopenapi.WithName(spec.Name),
		)
		if err != nil {
			return apierror.Internal(apierror.DomainTool, fmt.Sprintf("openapi %s: %s", spec.Name, err.Error()))
		}
		ac.out.ToolSets = append(ac.out.ToolSets, ts)
	}
	return nil
}

// assembleAgentTools creates agent-as-tool instances.
func (ac *assembleContext) assembleAgentTools() {
	for _, atCfg := range ac.cfg.AgentTools {
		opts := []trpcagenttool.Option{
			trpcagenttool.WithSkipSummarization(atCfg.SkipSummarization),
			trpcagenttool.WithStreamInner(atCfg.StreamInner),
		}
		if atCfg.Description != "" {
			opts = append(opts, trpcagenttool.WithDescription(atCfg.Description))
		}
		if atCfg.HistoryScope > 0 {
			opts = append(opts, trpcagenttool.WithHistoryScope(atCfg.HistoryScope))
		}
		if atCfg.ResponseMode > 0 {
			opts = append(opts, trpcagenttool.WithResponseMode(atCfg.ResponseMode))
		}
		t := trpcagenttool.NewTool(atCfg.Agent, opts...)
		ac.out.Tools = append(ac.out.Tools, t)
	}
}

// assembleMCPTools creates MCP server and broker tools.
func (ac *assembleContext) assembleMCPTools() error {
	// Collect acquired toolsets locally first: P1-2 schema governance below
	// may decide to drop them (degrade to broker), and error paths must
	// release pool references for anything not committed to ac.out.
	// mcpSets is nil-ed on every committed path; the deferred release only
	// fires for early error returns.
	var mcpSets []ToolSet
	defer func() {
		for _, ts := range mcpSets {
			_ = ts.Close() // pooled wrapper: decrements ref count
		}
	}()

	for _, mcpCfg := range ac.cfg.MCP.Servers {
		// Route through the process-level pool: identical connection configs
		// share one live MCP session across agent builds, so a build-cache
		// miss no longer pays a full reconnect (process spawn / TCP handshake
		// + initialize + tools/list). Credential-injected configs bypass the
		// pool automatically (see mcp_pool.go).
		ts, err := acquireMCPToolSet(ac.ctx, mcpCfg)
		if err != nil {
			return apierror.Internal(apierror.DomainTool, fmt.Sprintf("mcp %s: %s", mcpCfg.Name, err.Error()))
		}
		if ts != nil {
			// Probe the session so connection failures are visible via
			// loggateway at build time. We do NOT fail the assembly — the
			// ToolSet is still added and will retry on the next Tools() call,
			// preserving the Always-Ready Agent resilience semantics. On a
			// pooled, already-connected session this probe is a cheap no-op
			// list refresh.
			if init, ok := ts.(interface{ Init(context.Context) error }); ok {
				if initErr := init.Init(ac.ctx); initErr != nil {
					ac.lg.Warn("MCP ToolSet 初始化失败（降级运行，将在下次调用时重试）",
						loggateway.Domain("tools.mcp"),
						loggateway.Str("server", mcpCfg.Name),
						loggateway.Err(initErr))
				}
			}
			mcpSets = append(mcpSets, ts)
		}
	}

	brokerAdded := false
	if len(mcpSets) > 0 && ac.cfg.SkipMCPGovernance {
		// P0-2 阶段A 分片路径：片内跳过治理（截断+总预算降级是跨 server
		// 决策），原样提交直连 toolset；合并期由调用方对并集统一治理。
		ac.out.ToolSets = append(ac.out.ToolSets, mcpSets...)
		mcpSets = nil
	} else if len(mcpSets) > 0 {
		// P1-2 MCP schema 治理：截断 + 总预算；超预算降级 broker。
		gov := GovernMCPServerToolSets(ac.ctx, mcpSets, ac.lg)
		if gov.TruncatedCount > 0 {
			ac.lg.Info("MCP schema 治理：截断超长 declaration",
				loggateway.Domain("tools.mcp"),
				loggateway.Int("tool_count", gov.ToolCount),
				loggateway.Int("truncated_count", gov.TruncatedCount),
				loggateway.Int("total_chars", gov.TotalChars))
		}
		if gov.Degraded {
			brokerCfg := ac.cfg.MCP.Broker
			if brokerCfg == nil {
				brokerCfg = ac.cfg.MCP.BrokerFallback
			}
			if brokerCfg != nil {
				// 释放直连 toolset 的池引用，改用 broker（schema 按需拉取）。
				// K3 降级：进程日志 Warn。tools 层无 FlowLogWriter 端口（红线 3），
				// 与 v2 invariant_check 同先例，流程日志支路暂缓。
				for _, ts := range gov.Kept {
					_ = ts.Close()
				}
				mcpSets = nil
				brokerTools, err := buildMCPBrokerTools(*brokerCfg)
				if err != nil {
					return apierror.Internal(apierror.DomainTool, "mcpbroker: "+err.Error())
				}
				ac.out.Tools = append(ac.out.Tools, brokerTools...)
				brokerAdded = true
				ac.lg.Warn("MCP schema 总量超预算，直连模式降级为 broker",
					loggateway.Domain("tools.mcp"),
					loggateway.Int("tool_count", gov.ToolCount),
					loggateway.Int("total_chars", gov.TotalChars),
					loggateway.Int("budget_chars", mcpSchemaTotalBudgetChars))
			} else {
				// 无 broker 可降级：保留截断后的直连工具（best-effort）。
				ac.lg.Warn("MCP schema 总量超预算且无 broker 配置，保留截断后的直连工具",
					loggateway.Domain("tools.mcp"),
					loggateway.Int("tool_count", gov.ToolCount),
					loggateway.Int("total_chars", gov.TotalChars),
					loggateway.Int("budget_chars", mcpSchemaTotalBudgetChars))
				ac.out.ToolSets = append(ac.out.ToolSets, gov.Kept...)
				mcpSets = nil
			}
		} else {
			ac.out.ToolSets = append(ac.out.ToolSets, gov.Kept...)
			mcpSets = nil
		}
	}

	if !brokerAdded && ac.enabled["mcpbroker"] && ac.cfg.MCP.Broker != nil {
		brokerTools, err := buildMCPBrokerTools(*ac.cfg.MCP.Broker)
		if err != nil {
			return apierror.Internal(apierror.DomainTool, "mcpbroker: "+err.Error())
		}
		ac.out.Tools = append(ac.out.Tools, brokerTools...)
	}

	return nil
}

// assembleSessionTools creates memory, custom, message, and subagent tools.
func (ac *assembleContext) assembleSessionTools() {
	if len(ac.cfg.Session.MemoryTools) > 0 {
		ac.out.Tools = append(ac.out.Tools, ac.cfg.Session.MemoryTools...)
	} else if ac.cfg.Session.MemoryEnabled {
		ac.out.Tools = append(ac.out.Tools, memorytool.DefaultTools()...)
		ac.out.Tools = append(ac.out.Tools, memorytool.AdvancedTools()...)
	}

	ac.out.Tools = append(ac.out.Tools, ac.cfg.Session.CustomTools...)

	if ac.enabled["message"] && ac.cfg.Session.OutboundRouter != nil {
		mt := outbound.NewMessageTool(ac.cfg.Session.OutboundRouter)
		ac.out.Tools = append(ac.out.Tools, mt)
	}

	if ac.cfg.Session.SubAgentService != nil {
		ac.assembleSubagentTools()
	}

	// Client tool bridge: the ToolSet delegates execution to the desktop
	// companion via the process-wide bridge singleton (design 74 §6).
	if ac.enabled["client"] && ac.cfg.Session.ClientBridge != nil {
		ac.out.ToolSets = append(ac.out.ToolSets, clientbridge.NewToolSet(ac.cfg.Session.ClientBridge))
	}

	// Coding agent bridge: dispatch/check/cancel external coding CLI tasks
	// (design 76 §13). Tools read session from the invocation context.
	if ac.enabled["coding"] && ac.cfg.Session.CodingBridge != nil {
		ac.out.ToolSets = append(ac.out.ToolSets, codingbridge.NewToolSet(ac.cfg.Session.CodingBridge))
	}
}

// assembleSubagentTools adds enabled subagent framework tools.
func (ac *assembleContext) assembleSubagentTools() {
	anySubagent := ac.enabled["subagents_spawn"] || ac.enabled["subagents_list"] ||
		ac.enabled["subagents_get"] || ac.enabled["subagents_cancel"]
	if !anySubagent {
		return
	}
	for _, t := range ac.cfg.Session.SubAgentService.FrameworkTools() {
		if t == nil || t.Declaration() == nil {
			continue
		}
		if !ac.enabled[t.Declaration().Name] {
			continue
		}
		ac.out.Tools = append(ac.out.Tools, t)
	}
}

// assembleBrowserToolset creates the browser MCP toolset wrapped with a
// navigation SSRF guard. The guard validates URLs in browser_navigate calls
// against PlaywrightMCPConfig.Navigation before forwarding to the MCP server.
// When EnabledSubGroups is configured, a FilteringToolSet wraps the guarded
// set so only tools in the allowed sub-groups are exposed.
func (ac *assembleContext) assembleBrowserToolset() error {
	if !ac.enabled["browser"] || ac.cfg.Browser == nil {
		return nil
	}

	bcfg := ac.cfg.Browser
	mcpCfg := MCPServerConfig{
		Name:       "browser",
		Transport:  bcfg.Transport,
		ServerURL:  bcfg.ServerURL,
		Command:    bcfg.Command,
		Args:       bcfg.BuildArgs(),
		TimeoutSec: bcfg.TimeoutSec,
	}
	ts, err := acquireMCPToolSet(ac.ctx, mcpCfg)
	if err != nil {
		return apierror.Internal(apierror.DomainTool, "browser mcp: "+err.Error())
	}
	if ts == nil {
		return nil
	}
	policy := bcfg.EffectiveNavigationPolicy()
	var guarded trpctool.ToolSet = browser.NewNavigationGuardedToolSet(ts, policy)
	if len(bcfg.EnabledSubGroups) > 0 {
		guarded = browser.NewFilteringToolSet(guarded, bcfg.EnabledSubGroups)
	}
	ac.out.ToolSets = append(ac.out.ToolSets, guarded)
	return nil
}

// assembleBlobAndResultTools creates read_tool_result and deferred tools.
func (ac *assembleContext) assembleBlobAndResultTools() error {
	if ac.cfg.Session.BlobReader != nil && ac.enabled["read_tool_result"] {
		rt := custom.NewReadToolResultTool(ac.cfg.Session.BlobReader)
		ac.out.Tools = append(ac.out.Tools, rt)
	}

	if len(ac.cfg.DeferredTools) > 0 {
		return ac.assembleDeferredTools()
	}
	return nil
}

// assembleDeferredTools delegates to FinalizeDeferredTools (exported for the
// P0-2 阶段A shard merge path, which assembles shards without deferred
// processing and finalizes once over the union).
func (ac *assembleContext) assembleDeferredTools() error {
	return FinalizeDeferredTools(ac.ctx, ac.out, ac.cfg.DeferredTools, ac.lg)
}

// FinalizeDeferredTools builds the deferred tool catalog and wraps deferred
// tools with DeferredCallableTool. All tools must be fully assembled at this
// point (WP-4 修复版：全量装配 + wrapDeferred)。
//
// 流程：
//  1. 从 deferredTools（registry 名称）构建延迟注册表名集合
//  2. 扫描已装配的 ToolSets 和独立工具，匹配延迟注册表名
//  3. 构建 catalog（name + description + category）供 tool_search/tool_load
//  4. 注册工具引用到 manager（供 tool_load 返回完整 schema）
//  5. 包装延迟 ToolSet（DeferredToolSet）和独立工具（DeferredCallableTool）
//
// 对 out 的就地修改仅为切片元素替换（包装器），被包装工具本身不被变异，
// 因此可安全作用于分片缓存共享的产物并集。
func FinalizeDeferredTools(ctx context.Context, out *AssembledToolsets, deferredTools []string, lg loggateway.Logger) error {
	if len(deferredTools) == 0 || out == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}

	deferredRegNames := make(map[string]bool, len(deferredTools))
	for _, name := range deferredTools {
		deferredRegNames[name] = true
	}

	// 第一遍：构建 catalog + 收集工具引用
	// deferredRuntimeNames：LLM 可见的运行时名（catalog/filter/tool_load 用）
	// deferredBaseNames：原始声明名（DeferredToolSet/DeferredCallableTool 包装匹配用）
	var catalog []deferred.DeferredToolEntry
	deferredRuntimeNames := make(map[string]bool)
	deferredBaseNames := make(map[string]bool)
	toolRefs := make(map[string]trpctool.Tool)

	// 扫描 ToolSets：名称匹配延迟注册表名的 ToolSet，其所有工具均为延迟工具
	//
	// catalog 使用运行时名（"{toolset}_{tool}"，与框架 NamedTool 前缀约定一致），
	// 这是 LLM 在 tools block 中看到并调用的名字；BaseName 保留原始声明名，
	// 供 DeferredCallableTool 激活门禁匹配。
	for _, ts := range out.ToolSets {
		if ts == nil || !deferredRegNames[ts.Name()] {
			continue
		}
		cat := findRegistryCategory(ts.Name())
		tsName := ts.Name()
		for _, t := range ts.Tools(ctx) {
			if t == nil || t.Declaration() == nil {
				continue
			}
			baseName := t.Declaration().Name
			runtimeName := baseName
			if tsName != "" {
				runtimeName = tsName + "_" + baseName
			}
			if deferredRuntimeNames[runtimeName] {
				continue
			}
			deferredRuntimeNames[runtimeName] = true
			deferredBaseNames[baseName] = true
			catalog = append(catalog, deferred.DeferredToolEntry{
				Name:        runtimeName,
				BaseName:    baseName,
				Description: t.Declaration().Description,
				Category:    cat,
			})
			toolRefs[runtimeName] = t
		}
	}

	// 扫描独立工具：名称匹配延迟注册表名的工具（无 toolset 前缀）
	for _, t := range out.Tools {
		if t == nil || t.Declaration() == nil {
			continue
		}
		name := t.Declaration().Name
		if !deferredRegNames[name] || deferredRuntimeNames[name] {
			continue
		}
		deferredRuntimeNames[name] = true
		deferredBaseNames[name] = true
		catalog = append(catalog, deferred.DeferredToolEntry{
			Name:        name,
			BaseName:    name,
			Description: t.Declaration().Description,
			Category:    findRegistryCategory(name),
		})
		toolRefs[name] = t
	}

	if len(catalog) == 0 {
		return nil
	}

	// 创建 manager 和元工具
	searchTool := deferred.NewToolSearchTool(catalog)
	manager := searchTool.Manager()
	for name, t := range toolRefs {
		manager.RegisterTool(name, t)
	}
	loadTool := deferred.NewToolLoadToolWithManager(manager)
	out.Tools = append(out.Tools, searchTool, loadTool)
	out.DeferredManager = manager

	// 第二遍：包装延迟 ToolSets 和独立工具。
	// DeferredToolSet 内部比较的是 inner.Tools() 返回的原始声明名，因此用 baseName。
	for i, ts := range out.ToolSets {
		if ts == nil || !deferredRegNames[ts.Name()] {
			continue
		}
		out.ToolSets[i] = deferred.NewDeferredToolSet(ts, deferredBaseNames, lg)
	}
	for i, t := range out.Tools {
		if t == nil || t.Declaration() == nil {
			continue
		}
		if deferredBaseNames[t.Declaration().Name] {
			out.Tools[i] = deferred.NewDeferredCallableTool(t, lg)
		}
	}

	lg.Info("两段式工具加载：延迟工具装配完成",
		loggateway.StepID("tool.assemble.deferred_done"),
		loggateway.Int("deferred_count", len(catalog)),
		loggateway.Int("deferred_toolsets", countDeferredToolSets(out.ToolSets, deferredRegNames)),
	)
	return nil
}

// findRegistryCategory 查找注册表中指定名称的分类。
func findRegistryCategory(name string) string {
	for _, reg := range Registry() {
		if reg.Name == name {
			return reg.Category
		}
	}
	return ""
}

// dedupFlatToolNames delegates to DedupFlatToolNames (exported for the P0-2
// 阶段A shard merge path, which dedups once over the cross-shard union).
func (ac *assembleContext) dedupFlatToolNames() {
	DedupFlatToolNames(ac.ctx, ac.out, ac.lg)
}

// DedupFlatToolNames enforces earlier-wins over the flat tool list. Duplicate
// declaration names (e.g. two CustomTools injected by different layers) must
// not reach the model twice — most LLM APIs reject duplicate tool names.
// The first occurrence wins and later flat duplicates are dropped with a Warn.
//
// Cross collisions between flat tools and ToolSet members are detected (Warn
// only) for the cheap static toolsets in aliasExpandableToolSetNames; MCP and
// other dynamic toolsets are not enumerated here because their Tools() call
// may trigger a network roundtrip.
func DedupFlatToolNames(ctx context.Context, out *AssembledToolsets, lg loggateway.Logger) {
	if out == nil || len(out.Tools) == 0 {
		return
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	seen := make(map[string]struct{}, len(out.Tools))
	kept := make([]Tool, 0, len(out.Tools))
	for _, t := range out.Tools {
		if t == nil || t.Declaration() == nil {
			kept = append(kept, t)
			continue
		}
		name := t.Declaration().Name
		if _, dup := seen[name]; dup {
			lg.Warn("tools.assemble.duplicate_tool_name",
				loggateway.StepID("tool.assemble.duplicate_tool_name"),
				loggateway.Str("tool", name),
				loggateway.Str("resolution", "kept first occurrence, dropped later flat duplicate"))
			continue
		}
		seen[name] = struct{}{}
		kept = append(kept, t)
	}
	out.Tools = kept

	for _, ts := range out.ToolSets {
		if ts == nil || !aliasExpandableToolSetNames[ts.Name()] {
			continue
		}
		for _, t := range ts.Tools(ctx) {
			if t == nil || t.Declaration() == nil {
				continue
			}
			name := t.Declaration().Name
			if _, ok := seen[name]; ok {
				lg.Warn("tools.assemble.duplicate_tool_name",
					loggateway.StepID("tool.assemble.duplicate_tool_name"),
					loggateway.Str("tool", name),
					loggateway.Str("toolset", ts.Name()),
					loggateway.Str("resolution", "flat tool shadows toolset member on the model surface"))
			}
		}
	}
}

// countDeferredToolSets 统计被延迟的 ToolSet 数量。
func countDeferredToolSets(toolSets []ToolSet, deferredRegNames map[string]bool) int {
	count := 0
	for _, ts := range toolSets {
		if ts != nil && deferredRegNames[ts.Name()] {
			count++
		}
	}
	return count
}
