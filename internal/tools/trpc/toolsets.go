package trpc

import (
	"context"
	"fmt"

	"aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/internal/outbound"
	"aranea-agents/internal/sandbox"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/browser"
	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/internal/tools/codingbridge"
	computerusepkg "aranea-agents/internal/tools/computeruse"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	knowledgepkg "aranea-agents/internal/tools/knowledge"
	sandboxfspkg "aranea-agents/internal/tools/sandboxfs"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	subagenttool "aranea-agents/internal/tools/subagent"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"

	"aranea-agents/pkg/apierror"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ReplyFunc is re-exported so callers don't need to import serviceawaitreply directly.
type ReplyFunc = serviceawaitreply.ReplyFunc

type ToolsetConfig struct {
	Filesystem        bool
	FilesystemDir     string
	ShellExec         bool
	ShellExecDir      string
	ShellExecEnv      map[string]string
	WebFetch          bool
	WebSearch         bool
	WebResearch       bool
	WebResearchCfg    webresearchpkg.Config
	GeminiFetch       bool
	GeminiModel       string
	GoogleSearch      bool
	GoogleAPIKey      string
	GoogleCX          string
	ArxivSearch       bool
	Wikipedia         bool
	Email             bool
	Todo              bool
	AwaitReply        bool
	AwaitHook         ReplyFunc
	ClaudeCode        bool
	ClaudeCodeDir     string
	OpenAPISpecs      []OpenAPISpecConfig
	AgentTools        []AgentToolConfig
	MCPServers        []MCPServerConfig
	MCPBroker         *MCPBrokerConfig
	MCPBrokerFallback *MCPBrokerConfig
	CustomTools       []trpctool.Tool
	KnowledgeSearch   bool
	KnowledgeReflect  bool
	CallAgent         bool
	Kanban            bool
	KanbanBridge      kanbanpkg.Bridge
	MemoryEnabled     bool
	MemoryTools       []trpctool.Tool
	DeferredTools     []string
	BlobReader        biz.ToolResultBlobReader
	ReadDocument      bool
	ReadSpreadsheet   bool
	ReadLints         bool
	DeleteFile        bool
	WorkingMemory     bool
	Datetime          bool
	Message           bool
	OutboundRouter    *outbound.Router
	SubAgent          bool
	SubAgentService   *subagenttool.Service
	// ClientBridge enables the client tool bridge ToolSet (client_open_app /
	// client_open_url). Requires ClientBridgeSvc; when nil the flag is pruned
	// so agents never see a tool that would always fail offline.
	ClientBridge    bool
	ClientBridgeSvc *clientbridge.Bridge
	Browser         *browser.PlaywrightMCPConfig
	BrowserEnabled  bool
	// ComputerUse enables the desktop GUI automation toolset (computer_use_*).
	// Requires ComputerUseUC; when nil the flag is pruned so agents never see
	// a tool whose sidecar backend is unavailable (75-computer-use).
	ComputerUse   bool
	ComputerUseUC *bizcu.ComputerUseUsecase
	// CodingBridge enables the coding agent bridge ToolSet (coding_dispatch_task /
	// coding_check_task / coding_cancel_task). Requires CodingBridgeSvc; when nil
	// the flag is pruned so agents never see a tool with no backend (76-coding-agent-bridge).
	CodingBridge    bool
	CodingBridgeSvc codingbridge.BridgeService
	// SandboxFS enables the session-sandbox file toolset (sandbox_fs_write /
	// sandbox_fs_read, M82 P1-2). Requires SandboxFSStore (the process-wide
	// shared session-lease store, also bound to the codeexecutor sandbox
	// backend); when nil the flag is pruned so agents never see tools whose
	// sandbox backend is unavailable.
	SandboxFS      bool
	SandboxFSStore *sandbox.SessionLeases
	// SkipMCPGovernance（P0-2 阶段A）：分片构建时跳过片内 MCP schema 治理，
	// 由合并期对直连 toolset 并集统一执行。仅分片装配路径设置。
	SkipMCPGovernance bool
	// SkipPostProcess（P0-2 阶段A）：分片构建时跳过去重/消歧/别名横切处理，
	// 由合并期对跨分片并集统一重放。仅分片装配路径设置。
	SkipPostProcess bool
}

type AgentToolConfig = tools.AgentToolConfig

type MCPServerConfig = tools.MCPServerConfig

type MCPBrokerConfig = tools.MCPBrokerConfig

type OpenAPISpecConfig = tools.OpenAPISpecConfig

type AssembledToolsets = tools.AssembledToolsets

func BuildToolsets(ctx context.Context, cfg ToolsetConfig, lg loggateway.Logger) (*AssembledToolsets, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	enabled := []string{}
	if cfg.Filesystem {
		enabled = append(enabled, "file")
	}
	if cfg.ShellExec {
		enabled = append(enabled, "hostexec")
	}
	if cfg.WebFetch {
		enabled = append(enabled, "httpfetch")
	}
	if cfg.GeminiFetch {
		enabled = append(enabled, "geminifetch")
	}
	if cfg.WebSearch {
		enabled = append(enabled, "duckduckgo")
	}
	if cfg.GoogleSearch {
		enabled = append(enabled, "google_search")
	}
	if cfg.ArxivSearch {
		enabled = append(enabled, "arxiv_search")
	}
	if cfg.Wikipedia {
		enabled = append(enabled, "wikipedia")
	}
	if cfg.Email {
		enabled = append(enabled, "email")
	}
	if cfg.Todo {
		enabled = append(enabled, "todo")
	}
	if cfg.AwaitReply && cfg.AwaitHook == nil {
		// AwaitHook absent: use the framework's built-in tool (marks routing state only).
		// When AwaitHook is set, the ServiceTool is added to customTools below.
		enabled = append(enabled, "await_user_reply")
	}
	if cfg.ClaudeCode {
		enabled = append(enabled, "claudecode")
	}
	if len(cfg.AgentTools) > 0 {
		enabled = append(enabled, "agent")
	}
	if len(cfg.MCPServers) > 0 {
		enabled = append(enabled, "mcp")
	}
	if cfg.MCPBroker != nil {
		enabled = append(enabled, "mcpbroker")
	}
	if cfg.ReadDocument {
		enabled = append(enabled, "read_document")
	}
	if cfg.ReadSpreadsheet {
		enabled = append(enabled, "read_spreadsheet")
	}
	if cfg.ReadLints {
		enabled = append(enabled, "read_lints")
	}
	if cfg.DeleteFile {
		enabled = append(enabled, "delete_file")
	}
	if cfg.WorkingMemory {
		enabled = append(enabled, "working_memory")
	}
	if cfg.Datetime {
		enabled = append(enabled, "datetime")
	}
	if cfg.Message {
		enabled = append(enabled, "message")
	}
	if cfg.BrowserEnabled && cfg.Browser != nil {
		enabled = append(enabled, "browser")
	}
	if cfg.ClientBridge && cfg.ClientBridgeSvc != nil {
		enabled = append(enabled, clientbridge.ToolSetName)
	}
	if cfg.CodingBridge && cfg.CodingBridgeSvc != nil {
		enabled = append(enabled, codingbridge.ToolSetName)
	}

	openAPISpecs := make([]tools.OpenAPISpecConfig, len(cfg.OpenAPISpecs))
	for i, spec := range cfg.OpenAPISpecs {
		openAPISpecs[i] = tools.OpenAPISpecConfig{
			Name:     spec.Name,
			SpecURL:  spec.SpecURL,
			SpecData: spec.SpecData,
		}
	}

	agentTools := make([]tools.AgentToolConfig, len(cfg.AgentTools))
	for i, at := range cfg.AgentTools {
		agentTools[i] = tools.AgentToolConfig(at)
	}

	mcpServers := make([]tools.MCPServerConfig, len(cfg.MCPServers))
	for i, mcp := range cfg.MCPServers {
		mcpServers[i] = tools.MCPServerConfig(mcp)
	}

	var mcpBroker *tools.MCPBrokerConfig
	if cfg.MCPBroker != nil {
		b := tools.MCPBrokerConfig(*cfg.MCPBroker)
		mcpBroker = &b
	}
	var mcpBrokerFallback *tools.MCPBrokerConfig
	if cfg.MCPBrokerFallback != nil {
		b := tools.MCPBrokerConfig(*cfg.MCPBrokerFallback)
		mcpBrokerFallback = &b
	}

	customTools := make([]tools.Tool, len(cfg.CustomTools))
	for i, t := range cfg.CustomTools {
		customTools[i] = t
	}
	if cfg.KnowledgeSearch {
		customTools = append(customTools, knowledgepkg.NewSearchTool(lg))
	}
	if cfg.KnowledgeReflect {
		customTools = append(customTools, knowledgepkg.NewReflectTool(lg))
	}
	if cfg.WebResearch {
		t, err := webresearchpkg.NewTool(cfg.WebResearchCfg, lg)
		if err != nil {
			return nil, apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("web_research: %s", err.Error()))
		}
		customTools = append(customTools, t)
	}
	if cfg.CallAgent {
		customTools = append(customTools, a2a.NewCallAgentTool())
	}
	if cfg.AwaitReply && cfg.AwaitHook != nil {
		// EP-RT-02: use the service-integrated tool that blocks mid-turn so the
		// user reply is delivered back to the agent via the awaitChans channel.
		customTools = append(customTools, serviceawaitreply.New(lg))
	}
	if cfg.Kanban {
		for _, t := range kanbanpkg.NewToolset(cfg.KanbanBridge) {
			customTools = append(customTools, t)
		}
	}
	if cfg.ComputerUse && cfg.ComputerUseUC != nil {
		for _, t := range computerusepkg.NewToolset(cfg.ComputerUseUC) {
			customTools = append(customTools, t)
		}
	}
	if cfg.SandboxFS && cfg.SandboxFSStore != nil {
		for _, t := range sandboxfspkg.NewToolset(cfg.SandboxFSStore) {
			customTools = append(customTools, t)
		}
	}

	// Only enable subagent tools when the agent's effective tools include them
	// AND the SubAgentService is actually wired. This prevents every agent from
	// gaining subagent capability just because the service is available.
	if cfg.SubAgent && cfg.SubAgentService != nil {
		enabled = append(enabled, "subagents_spawn", "subagents_list", "subagents_get", "subagents_cancel")
		lg.Info("subagent tools enabled",
			loggateway.StepID("tool.subagent_enabled"),
			loggateway.Bool("subagent_service_wired", cfg.SubAgentService != nil))
	}

	assembled, err := tools.Assemble(ctx, tools.AssemblyConfig{
		EnabledTools:  enabled,
		DeferredTools: cfg.DeferredTools,
		FilesystemDir: cfg.FilesystemDir,
		ShellExec: tools.ShellExecConfig{
			Dir: cfg.ShellExecDir,
			Env: cfg.ShellExecEnv,
		},
		Search: tools.SearchConfig{
			GeminiModel:  cfg.GeminiModel,
			GoogleAPIKey: cfg.GoogleAPIKey,
			GoogleCX:     cfg.GoogleCX,
		},
		ClaudeCode: tools.ClaudeCodeConfig{
			Dir: cfg.ClaudeCodeDir,
		},
		OpenAPISpecs: openAPISpecs,
		AgentTools:   agentTools,
		MCP: tools.MCPConfig{
			Servers:        mcpServers,
			Broker:         mcpBroker,
			BrokerFallback: mcpBrokerFallback,
		},
		Session: tools.SessionConfig{
			MemoryEnabled:   cfg.MemoryEnabled,
			MemoryTools:     cfg.MemoryTools,
			CustomTools:     customTools,
			OutboundRouter:  cfg.OutboundRouter,
			SubAgentService: cfg.SubAgentService,
			BlobReader:      cfg.BlobReader,
			ClientBridge:    cfg.ClientBridgeSvc,
			CodingBridge:    cfg.CodingBridgeSvc,
		},
		Browser: cfg.Browser,
		Lg:      lg,

		SkipMCPGovernance: cfg.SkipMCPGovernance,
		SkipPostProcess:   cfg.SkipPostProcess,
	})
	if err != nil {
		return nil, err
	}
	if !cfg.SkipPostProcess {
		tools.ApplyRuntimeNameAliases(ctx, assembled)
	}
	return assembled, nil
}
