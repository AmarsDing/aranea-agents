package trpc

import (
	"context"
	"fmt"

	"aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/outbound"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/browser"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	knowledgepkg "aranea-agents/internal/tools/knowledge"
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
	Filesystem       bool
	FilesystemDir    string
	ShellExec        bool
	ShellExecDir     string
	ShellExecEnv     map[string]string
	WebFetch         bool
	WebSearch        bool
	WebResearch      bool
	WebResearchCfg   webresearchpkg.Config
	GeminiFetch      bool
	GeminiModel      string
	GoogleSearch     bool
	GoogleAPIKey     string
	GoogleCX         string
	ArxivSearch      bool
	Wikipedia        bool
	Email            bool
	Todo             bool
	AwaitReply       bool
	AwaitHook        ReplyFunc
	ClaudeCode       bool
	ClaudeCodeDir    string
	OpenAPISpecs     []OpenAPISpecConfig
	WorkspaceExec    bool
	AgentTools       []AgentToolConfig
	MCPServers       []MCPServerConfig
	MCPBroker        *MCPBrokerConfig
	CustomTools      []trpctool.Tool
	KnowledgeSearch  bool
	KnowledgeReflect bool
	CallAgent        bool
	Kanban           bool
	KanbanBridge     kanbanpkg.Bridge
	MemoryEnabled    bool
	MemoryTools      []trpctool.Tool
	DeferredTools    []string
	BlobReader       biz.ToolResultBlobReader
	ReadDocument     bool
	ReadSpreadsheet  bool
	WorkingMemory    bool
	Datetime         bool
	Message          bool
	OutboundRouter   *outbound.Router
	SubAgent         bool
	SubAgentService  *subagenttool.Service
	Browser          *browser.PlaywrightMCPConfig
	BrowserEnabled   bool
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
	if cfg.WorkspaceExec {
		enabled = append(enabled, "workspace_exec")
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

	customTools := make([]tools.Tool, len(cfg.CustomTools))
	for i, t := range cfg.CustomTools {
		customTools[i] = t
	}
	if cfg.KnowledgeSearch {
		customTools = append(customTools, knowledgepkg.NewSearchTool())
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
			Servers: mcpServers,
			Broker:  mcpBroker,
		},
		Session: tools.SessionConfig{
			MemoryEnabled:   cfg.MemoryEnabled,
			MemoryTools:     cfg.MemoryTools,
			CustomTools:     customTools,
			OutboundRouter:  cfg.OutboundRouter,
			SubAgentService: cfg.SubAgentService,
			BlobReader:      cfg.BlobReader,
		},
		Browser: cfg.Browser,
		Lg:      lg,
	})
	if err != nil {
		return nil, err
	}
	tools.ApplyRuntimeNameAliases(ctx, assembled)
	return assembled, nil
}
