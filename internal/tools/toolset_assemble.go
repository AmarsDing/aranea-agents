package tools

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/outbound"
	"aranea-agents/internal/tools/custom"
	"aranea-agents/internal/tools/deferred"
	documentpkg "aranea-agents/internal/tools/document"
	hostexecpkg "aranea-agents/internal/tools/hostexec"
	memorytool "aranea-agents/internal/tools/memory"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
	trpcclaudecode "trpc.group/trpc-go/trpc-agent-go/tool/claudecode"
	trpcfile "trpc.group/trpc-go/trpc-agent-go/tool/file"
	trpcgeminifetch "trpc.group/trpc-go/trpc-agent-go/tool/webfetch/geminifetch"
	trpcgooglesearch "trpc.group/trpc-go/trpc-agent-go/tool/google/search"
	trpcopenapi "trpc.group/trpc-go/trpc-agent-go/tool/openapi"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
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

// assembleFromRegistry instantiates tools from the global registry entries.
func (ac *assembleContext) assembleFromRegistry() error {
	for _, reg := range Registry() {
		if !ac.enabled[reg.Name] {
			continue
		}
		if ac.deferredSet[reg.Name] {
			continue
		}
		if reg.ToolSetFactory != nil {
			ts, err := reg.ToolSetFactory(ac.ctx)
			if err != nil {
				return apierror.Internal(apierror.DomainTool, fmt.Sprintf("tool %s: %s", reg.Name, err.Error()))
			}
			if ts != nil {
				ac.out.ToolSets = append(ac.out.ToolSets, ts)
			} else {
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
			} else {
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
		ac.out.ToolSets = append(ac.out.ToolSets, ts)
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
	for _, mcpCfg := range ac.cfg.MCP.Servers {
		ts, err := buildMCPToolSet(mcpCfg)
		if err != nil {
			return apierror.Internal(apierror.DomainTool, fmt.Sprintf("mcp %s: %s", mcpCfg.Name, err.Error()))
		}
		if ts != nil {
			ac.out.ToolSets = append(ac.out.ToolSets, ts)
		}
	}

	if ac.enabled["mcpbroker"] && ac.cfg.MCP.Broker != nil {
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
	}

	ac.out.Tools = append(ac.out.Tools, ac.cfg.Session.CustomTools...)

	if ac.enabled["message"] && ac.cfg.Session.OutboundRouter != nil {
		mt := outbound.NewMessageTool(ac.cfg.Session.OutboundRouter)
		ac.out.Tools = append(ac.out.Tools, mt)
	}

	if ac.cfg.Session.SubAgentService != nil {
		ac.assembleSubagentTools()
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

// assembleBrowserToolset creates the browser MCP toolset.
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
	ts, err := buildMCPToolSet(mcpCfg)
	if err != nil {
		return apierror.Internal(apierror.DomainTool, "browser mcp: "+err.Error())
	}
	if ts != nil {
		ac.out.ToolSets = append(ac.out.ToolSets, ts)
	}
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

// assembleDeferredTools creates deferred tool entries and their callable wrappers.
func (ac *assembleContext) assembleDeferredTools() error {
	var catalog []deferred.DeferredToolEntry
	registryEntries := Registry()
	regByName := make(map[string]*ToolRegistration, len(registryEntries))
	for _, reg := range registryEntries {
		regByName[reg.Name] = reg
	}
	for _, name := range ac.cfg.DeferredTools {
		reg, ok := regByName[name]
		if !ok {
			ac.lg.Warn("tools.assemble.deferred_not_found",
				loggateway.StepID("tool.assemble.deferred_not_found"),
				loggateway.Str("tool", name),
				loggateway.Str("reason", "deferred tool not in registry"))
			continue
		}
		if reg.Factory != nil {
			catalog = append(catalog, deferred.DeferredToolEntry{
				Name:        reg.Name,
				Description: reg.Description,
				Category:    reg.Category,
				Factory:     reg.Factory,
			})
		} else if reg.ToolSetFactory != nil {
			// Expand ToolSetFactory into individual tool entries so all tools
			// in the set are accessible, not just the first one.
			expanded := ac.expandToolSetFactory(reg)
			catalog = append(catalog, expanded...)
		}
	}
	if len(catalog) == 0 {
		return nil
	}

	searchTool := deferred.NewToolSearchTool(catalog)
	ac.out.Tools = append(ac.out.Tools, searchTool)
	ac.out.DeferredManager = searchTool.Manager()

	for _, entry := range catalog {
		if entry.Factory == nil {
			continue
		}
		dt := deferred.NewDeferredCallableTool(
			&trpctool.Declaration{
				Name:        entry.Name,
				Description: entry.Description,
			},
			entry.Factory,
			ac.lg,
		)
		ac.out.Tools = append(ac.out.Tools, dt)
	}
	return nil
}

// expandToolSetFactory eagerly creates the toolset and returns one
// DeferredToolEntry per tool. This ensures all tools in a toolset are
// accessible as deferred tools, not just the first one.
func (ac *assembleContext) expandToolSetFactory(reg *ToolRegistration) []deferred.DeferredToolEntry {
	ts, err := reg.ToolSetFactory(ac.ctx)
	if err != nil {
		ac.lg.Warn("tools.assemble.deferred_toolset_factory_failed",
			loggateway.StepID("tool.assemble.deferred_toolset_factory_fail"),
			loggateway.Str("tool", reg.Name),
			loggateway.Err(err))
		return nil
	}
	if ts == nil {
		return nil
	}
	tools := ts.Tools(ac.ctx)
	if len(tools) == 0 {
		return nil
	}
	entries := make([]deferred.DeferredToolEntry, 0, len(tools))
	for _, t := range tools {
		if t == nil || t.Declaration() == nil {
			continue
		}
		tool := t // capture loop variable
		decl := tool.Declaration()
		entries = append(entries, deferred.DeferredToolEntry{
			Name:        decl.Name,
			Description: decl.Description,
			Category:    reg.Category,
			Factory: func(_ context.Context) (trpctool.Tool, error) {
				return tool, nil
			},
		})
	}
	return entries
}
