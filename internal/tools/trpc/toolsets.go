package trpc

import (
	"context"
	"fmt"

	"aranea-agents/internal/a2a"
	"aranea-agents/internal/tools"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	knowledgepkg "aranea-agents/internal/tools/knowledge"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ReplyFunc is re-exported so callers don't need to import serviceawaitreply directly.
type ReplyFunc = serviceawaitreply.ReplyFunc

type ToolsetConfig struct {
	Filesystem    bool
	FilesystemDir string
	ShellExec     bool
	ShellExecDir  string
	WebFetch      bool
	WebSearch     bool
	WebResearch   bool
	WebResearchCfg webresearchpkg.Config
	GeminiFetch   bool
	GeminiModel   string
	GoogleSearch  bool
	GoogleAPIKey  string
	GoogleCX      string
	ArxivSearch   bool
	Wikipedia     bool
	Email         bool
	Todo          bool
	// AwaitReply enables the await_user_reply tool.
	// When AwaitHook is also set the service-integrated ServiceTool is used
	// (blocks mid-turn and delivers the reply text back to the agent).
	// When AwaitHook is nil the framework's built-in tool is used (marks
	// routing state only; does not block).
	AwaitReply bool
	// AwaitHook is an optional blocking callback injected by the ChatService.
	// When non-nil, the ServiceTool replaces the framework's await_user_reply.
	AwaitHook       ReplyFunc
	ClaudeCode      bool
	ClaudeCodeDir   string
	OpenAPISpecs    []OpenAPISpecConfig
	WorkspaceExec   bool
	AgentTools      []AgentToolConfig
	MCPServers      []MCPServerConfig
	MCPBroker       *MCPBrokerConfig
	CustomTools     []trpctool.Tool
	KnowledgeSearch bool
	CallAgent       bool
	Kanban          bool
}

type AgentToolConfig = tools.AgentToolConfig

type MCPServerConfig = tools.MCPServerConfig

type MCPBrokerConfig = tools.MCPBrokerConfig

type OpenAPISpecConfig = tools.OpenAPISpecConfig

type AssembledToolsets = tools.AssembledToolsets

func BuildToolsets(ctx context.Context, cfg ToolsetConfig) (*AssembledToolsets, error) {
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
	if cfg.WebResearch {
		t, err := webresearchpkg.NewTool(cfg.WebResearchCfg)
		if err != nil {
			return nil, fmt.Errorf("web_research: %w", err)
		}
		customTools = append(customTools, t)
	}
	if cfg.CallAgent {
		customTools = append(customTools, a2a.NewCallAgentTool())
	}
	if cfg.AwaitReply && cfg.AwaitHook != nil {
		// EP-RT-02: use the service-integrated tool that blocks mid-turn so the
		// user reply is delivered back to the agent via the awaitChans channel.
		customTools = append(customTools, serviceawaitreply.New())
	}
	if cfg.Kanban {
		for _, t := range kanbanpkg.NewToolset() {
			customTools = append(customTools, t)
		}
	}

	assembled, err := tools.Assemble(ctx, tools.AssemblyConfig{
		EnabledTools:  enabled,
		FilesystemDir: cfg.FilesystemDir,
		ShellExecDir:  cfg.ShellExecDir,
		GeminiModel:   cfg.GeminiModel,
		GoogleAPIKey:  cfg.GoogleAPIKey,
		GoogleCX:      cfg.GoogleCX,
		ClaudeCodeDir: cfg.ClaudeCodeDir,
		OpenAPISpecs:  openAPISpecs,
		AgentTools:    agentTools,
		MCPServers:    mcpServers,
		MCPBroker:     mcpBroker,
		CustomTools:   customTools,
	})
	if err != nil {
		return nil, err
	}
	tools.ApplyRuntimeNameAliases(ctx, assembled)
	return assembled, nil
}
