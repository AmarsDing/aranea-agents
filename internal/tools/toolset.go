package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcagenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
	trpcarxivsearch "trpc.group/trpc-go/trpc-agent-go/tool/arxivsearch"
	trpcawaitreply "trpc.group/trpc-go/trpc-agent-go/tool/awaitreply"
	trpcclaudecode "trpc.group/trpc-go/trpc-agent-go/tool/claudecode"
	trpcduckduckgo "trpc.group/trpc-go/trpc-agent-go/tool/duckduckgo"
	trpcemail "trpc.group/trpc-go/trpc-agent-go/tool/email"
	trpcfile "trpc.group/trpc-go/trpc-agent-go/tool/file"
	trpcgooglesearch "trpc.group/trpc-go/trpc-agent-go/tool/google/search"
	trpchostexec "trpc.group/trpc-go/trpc-agent-go/tool/hostexec"
	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
	trpcmcpbroker "trpc.group/trpc-go/trpc-agent-go/tool/mcpbroker"
	trpcopenapi "trpc.group/trpc-go/trpc-agent-go/tool/openapi"
	trpctodo "trpc.group/trpc-go/trpc-agent-go/tool/todo"
	trpcgeminifetch "trpc.group/trpc-go/trpc-agent-go/tool/webfetch/geminifetch"
	trpchttpfetch "trpc.group/trpc-go/trpc-agent-go/tool/webfetch/httpfetch"
	trpcwikipedia "trpc.group/trpc-go/trpc-agent-go/tool/wikipedia"
	trpcworkspaceexec "trpc.group/trpc-go/trpc-agent-go/tool/workspaceexec"
)

var (
	registryOnce sync.Once
	registry     []*ToolRegistration
)

func Registry() []*ToolRegistration {
	registryOnce.Do(func() {
		registry = []*ToolRegistration{
			{
				Name:        "file",
				Description: "File operation ToolSet (read, write, search, replace, list)",
				Category:    "filesystem",
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcfile.NewToolSet()
				},
				EnabledByDefault:    true,
				RiskLevel:           "low",
				SupportsConcurrency: true,
			},
			{
				Name:        "hostexec",
				Description: "Host command execution ToolSet (shell, bash, powershell)",
				Category:    "execution",
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpchostexec.NewToolSet()
				},
				EnabledByDefault:     false,
				RiskLevel:            "critical",
				RequiresConfirmation: true,
			},
			{
				Name:        "httpfetch",
				Description: "HTTP web page fetch tool",
				Category:    "web",
				Factory: func(ctx context.Context) (Tool, error) {
					return trpchttpfetch.NewTool(), nil
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:        "claudefetch",
				Description: "Claude web fetch tool (Claude-powered page extraction) — framework stub, not yet implemented",
				Category:    "web",
				Factory: func(ctx context.Context) (Tool, error) {
					return nil, fmt.Errorf("claudefetch: framework stub, not yet implemented in trpc-agent-go")
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:        "geminifetch",
				Description: "Gemini web fetch tool (Gemini-powered page extraction)",
				Category:    "web",
				Factory: func(ctx context.Context) (Tool, error) {
					t, err := trpcgeminifetch.NewTool("")
					if err != nil {
						return nil, fmt.Errorf("geminifetch: %w", err)
					}
					return t, nil
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:        "duckduckgo",
				Description: "DuckDuckGo web search tool",
				Category:    "search",
				Factory: func(ctx context.Context) (Tool, error) {
					return trpcduckduckgo.NewTool(), nil
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:        "google_search",
				Description: "Google Custom Search ToolSet",
				Category:    "search",
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcgooglesearch.NewToolSet(ctx)
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:        "arxiv_search",
				Description: "ArXiv paper search ToolSet",
				Category:    "search",
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcarxivsearch.NewToolSet()
				},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "wikipedia",
				Description: "Wikipedia search ToolSet",
				Category:    "search",
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcwikipedia.NewToolSet()
				},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "email",
				Description: "Email sending ToolSet",
				Category:    "communication",
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcemail.NewToolSet()
				},
				EnabledByDefault:     false,
				RiskLevel:            "high",
				RequiresConfirmation: true,
			},
			{
				Name:        "todo",
				Description: "Todo management tool",
				Category:    "productivity",
				Factory: func(ctx context.Context) (Tool, error) {
					return trpctodo.New(), nil
				},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "await_user_reply",
				Description: "Mark agent as waiting for user reply",
				Category:    "interaction",
				Factory: func(ctx context.Context) (Tool, error) {
					return trpcawaitreply.New(), nil
				},
				EnabledByDefault: false,
				RiskLevel:        "low",
			},
			{
				Name:        "claudecode",
				Description: "Claude Code ToolSet (bash, edit, read, write, glob, grep, etc.)",
				Category:    "coding",
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return trpcclaudecode.NewToolSet()
				},
				EnabledByDefault:     false,
				RiskLevel:            "critical",
				RequiresConfirmation: true,
			},
			{
				Name:        "workspace_exec",
				Description: "Workspace execution tools (exec, write_stdin, kill_session)",
				Category:    "execution",
				Factory: func(ctx context.Context) (Tool, error) {
					return trpcworkspaceexec.NewExecTool(nil), nil
				},
				EnabledByDefault:     false,
				RiskLevel:            "critical",
				RequiresConfirmation: true,
			},
			{
				Name:        "openapi",
				Description: "OpenAPI spec ToolSet (dynamic REST API tools from spec)",
				Category:    "integration",
				ToolSetFactory: func(ctx context.Context) (ToolSet, error) {
					return nil, fmt.Errorf("openapi requires spec configuration")
				},
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "agent",
				Description:      "Agent-as-Tool: wrap an Agent as a callable Tool for delegation/composition",
				Category:         "composition",
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "mcp",
				Description:      "MCP ToolSet: connect to MCP servers (stdio/sse/streamable_http) and expose their tools",
				Category:         "integration",
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
			{
				Name:             "mcpbroker",
				Description:      "MCP Broker: runtime MCP discovery tools (mcp_list_servers, mcp_list_tools, mcp_inspect_tools, mcp_call)",
				Category:         "integration",
				EnabledByDefault: false,
				RiskLevel:        "medium",
			},
		}
	})
	return registry
}

type AgentToolConfig struct {
	Agent             trpcagent.Agent
	Name              string
	Description       string
	SkipSummarization bool
	StreamInner       bool
	HistoryScope      trpcagenttool.HistoryScope
	ResponseMode      trpcagenttool.ResponseMode
}

type MCPServerConfig struct {
	Name                string
	Transport           string
	ServerURL           string
	Command             string
	Args                []string
	Env                 map[string]string
	Headers             map[string]string
	TimeoutSec          int
	ToolPrefix          string
	SessionReconnectMax int
	AllowAdHocHTTP      bool
	AdHocTimeoutSec     int
}

type MCPBrokerConfig struct {
	Servers         []MCPServerConfig
	AllowAdHocHTTP  bool
	AdHocTimeoutSec int
}

type AssemblyConfig struct {
	EnabledTools  []string
	FilesystemDir string
	GeminiModel   string
	GoogleAPIKey  string
	GoogleCX      string
	ClaudeCodeDir string
	OpenAPISpecs  []OpenAPISpecConfig
	AgentTools    []AgentToolConfig
	MCPServers    []MCPServerConfig
	MCPBroker     *MCPBrokerConfig
	CustomTools   []Tool
}

type OpenAPISpecConfig struct {
	Name     string
	SpecURL  string
	SpecData []byte
}

type AssembledToolsets struct {
	ToolSets []ToolSet
	Tools    []Tool
}

func Assemble(ctx context.Context, cfg AssemblyConfig) (*AssembledToolsets, error) {
	out := &AssembledToolsets{}
	enabled := make(map[string]bool, len(cfg.EnabledTools))
	for _, name := range cfg.EnabledTools {
		enabled[name] = true
	}

	for _, reg := range Registry() {
		if !enabled[reg.Name] {
			continue
		}
		if reg.ToolSetFactory != nil {
			ts, err := reg.ToolSetFactory(ctx)
			if err != nil {
				return nil, fmt.Errorf("tool %s: %w", reg.Name, err)
			}
			if ts != nil {
				out.ToolSets = append(out.ToolSets, ts)
			}
		} else if reg.Factory != nil {
			t, err := reg.Factory(ctx)
			if err != nil {
				return nil, fmt.Errorf("tool %s: %w", reg.Name, err)
			}
			if t != nil {
				out.Tools = append(out.Tools, t)
			}
		}
	}

	if enabled["file"] && cfg.FilesystemDir != "" {
		ts, err := trpcfile.NewToolSet(trpcfile.WithBaseDir(cfg.FilesystemDir))
		if err != nil {
			return nil, fmt.Errorf("file toolset with dir: %w", err)
		}
		for i, existing := range out.ToolSets {
			if existing.Name() == "file" {
				out.ToolSets[i] = ts
				break
			}
		}
	}

	if enabled["geminifetch"] && cfg.GeminiModel != "" {
		t, err := trpcgeminifetch.NewTool(cfg.GeminiModel)
		if err != nil {
			return nil, fmt.Errorf("geminifetch: %w", err)
		}
		for i, existing := range out.Tools {
			if decl := existing.Declaration(); decl != nil && decl.Name == "gemini_web_fetch" {
				out.Tools[i] = t
				break
			}
		}
	}

	if enabled["google_search"] && cfg.GoogleAPIKey != "" && cfg.GoogleCX != "" {
		ts, err := trpcgooglesearch.NewToolSet(ctx,
			trpcgooglesearch.WithAPIKey(cfg.GoogleAPIKey),
			trpcgooglesearch.WithEngineID(cfg.GoogleCX),
		)
		if err != nil {
			return nil, fmt.Errorf("google search: %w", err)
		}
		for i, existing := range out.ToolSets {
			if existing.Name() == "google_search" {
				out.ToolSets[i] = ts
				break
			}
		}
	}

	if enabled["claudecode"] && cfg.ClaudeCodeDir != "" {
		ts, err := trpcclaudecode.NewToolSet(trpcclaudecode.WithBaseDir(cfg.ClaudeCodeDir))
		if err != nil {
			return nil, fmt.Errorf("claudecode: %w", err)
		}
		for i, existing := range out.ToolSets {
			if existing.Name() == "claudecode" {
				out.ToolSets[i] = ts
				break
			}
		}
	}

	for _, spec := range cfg.OpenAPISpecs {
		var specLoader trpcopenapi.Loader
		var err error
		if len(spec.SpecData) > 0 {
			specLoader, err = trpcopenapi.NewDataLoader(spec.SpecData)
		} else if spec.SpecURL != "" {
			specLoader, err = trpcopenapi.NewURILoader(spec.SpecURL)
		}
		if err != nil || specLoader == nil {
			continue
		}
		ts, err := trpcopenapi.NewToolSet(ctx,
			trpcopenapi.WithSpecLoader(specLoader),
			trpcopenapi.WithName(spec.Name),
		)
		if err != nil {
			return nil, fmt.Errorf("openapi %s: %w", spec.Name, err)
		}
		out.ToolSets = append(out.ToolSets, ts)
	}

	if enabled["workspace_exec"] {
		execTool := trpcworkspaceexec.NewExecTool(nil)
		for i, existing := range out.Tools {
			if decl := existing.Declaration(); decl != nil && decl.Name == "workspace_exec" {
				out.Tools[i] = execTool
				break
			}
		}
		out.Tools = append(out.Tools, trpcworkspaceexec.NewWriteStdinTool(execTool))
		out.Tools = append(out.Tools, trpcworkspaceexec.NewKillSessionTool(execTool))
	}

	for _, atCfg := range cfg.AgentTools {
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
		out.Tools = append(out.Tools, t)
	}

	for _, mcpCfg := range cfg.MCPServers {
		ts, err := buildMCPToolSet(mcpCfg)
		if err != nil {
			return nil, fmt.Errorf("mcp %s: %w", mcpCfg.Name, err)
		}
		if ts != nil {
			out.ToolSets = append(out.ToolSets, ts)
		}
	}

	if enabled["mcpbroker"] && cfg.MCPBroker != nil {
		brokerTools, err := buildMCPBrokerTools(*cfg.MCPBroker)
		if err != nil {
			return nil, fmt.Errorf("mcpbroker: %w", err)
		}
		out.Tools = append(out.Tools, brokerTools...)
	}

	out.Tools = append(out.Tools, cfg.CustomTools...)

	return out, nil
}

// DefaultMCPServerTimeoutSec is applied when config_json.timeout_sec is unset.
const DefaultMCPServerTimeoutSec = 60

func mcpTimeoutDuration(timeoutSec int) time.Duration {
	if timeoutSec <= 0 {
		timeoutSec = DefaultMCPServerTimeoutSec
	}
	return parseDurationSec(timeoutSec)
}

func buildMCPToolSet(cfg MCPServerConfig) (ToolSet, error) {
	transport := strings.TrimSpace(cfg.Transport)
	if transport == "" {
		transport = "stdio"
	}
	connCfg := trpcmcp.ConnectionConfig{
		Transport: transport,
		ServerURL: strings.TrimSpace(cfg.ServerURL),
		Headers:   cfg.Headers,
		Command:   strings.TrimSpace(cfg.Command),
		Args:      cfg.Args,
		Timeout:   mcpTimeoutDuration(cfg.TimeoutSec),
	}

	opts := []trpcmcp.ToolSetOption{trpcmcp.WithName(cfg.Name)}
	if pred := ToolFilterForPrefix(cfg.ToolPrefix); pred != nil {
		opts = append(opts, trpcmcp.WithToolFilterFunc(pred))
	}
	if cfg.SessionReconnectMax > 0 {
		opts = append(opts, trpcmcp.WithSessionReconnect(cfg.SessionReconnectMax))
	}

	return trpcmcp.NewMCPToolSet(connCfg, opts...), nil
}

func buildMCPBrokerTools(cfg MCPBrokerConfig) ([]Tool, error) {
	servers := make(map[string]trpcmcp.ConnectionConfig, len(cfg.Servers))
	for _, s := range cfg.Servers {
		transport := strings.TrimSpace(s.Transport)
		if transport == "" {
			transport = "stdio"
		}
		connCfg := trpcmcp.ConnectionConfig{
			Transport: transport,
			ServerURL: strings.TrimSpace(s.ServerURL),
			Headers:   s.Headers,
			Command:   strings.TrimSpace(s.Command),
			Args:      s.Args,
			Timeout:   mcpTimeoutDuration(s.TimeoutSec),
		}
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		servers[name] = connCfg
	}

	brokerOpts := []trpcmcpbroker.Option{}
	if len(servers) > 0 {
		brokerOpts = append(brokerOpts, trpcmcpbroker.WithServers(servers))
	}
	if cfg.AllowAdHocHTTP {
		brokerOpts = append(brokerOpts, trpcmcpbroker.WithAllowAdHocHTTP(true))
	}
	brokerOpts = append(brokerOpts, trpcmcpbroker.WithAdHocHTTPTimeout(mcpTimeoutDuration(cfg.AdHocTimeoutSec)))

	broker := trpcmcpbroker.New(brokerOpts...)
	tools := broker.Tools()
	result := make([]Tool, len(tools))
	for i, t := range tools {
		result[i] = t
	}
	return result, nil
}

func ToolFilterForPrefix(prefix string) ToolFilterFunc {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	return func(_ context.Context, t Tool) bool {
		if t == nil || t.Declaration() == nil {
			return false
		}
		return strings.HasPrefix(t.Declaration().Name, prefix)
	}
}

type ToolFilterFunc = func(context.Context, Tool) bool

func parseDurationSec(sec int) time.Duration {
	return time.Duration(sec) * time.Second
}
